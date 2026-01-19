package network

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"hash/crc32"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type rawconn interface {
	ID() *packet.Identity
	IP() string
	Close() error
	WriteData(data []byte) error
}
type Transport interface {
	Close(ctx context.Context) error
	Listen(addr string) error
	OnClose(handler func(c Conn))
	OnPacket(handler func(p packet.Packet, c Conn))
	OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode)
}
type Conn interface {
	ID() *packet.Identity
	IP() string
	Close() error
	IsIdle() bool
	OnClose(handler func())
	IsClosed() bool
	SendPong() error
	RecvPong()
	SendPing(ctx context.Context) error
	SendPacket(ctx context.Context, p packet.Packet) error
	CloseClode(code packet.CloseCode) error
	ConnackCode(code packet.ConnectCode, connId string) error
	SendMessage(ctx context.Context, p *packet.Message) error
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
	RecvMessack(p *packet.Messack)
	RecvResponse(p *packet.Response)
}
type conn struct {
	raw          rawconn
	logger       *xlog.Logger
	closed       atomic.Bool
	pingLock     sync.Mutex
	pingChan     chan struct{}
	sendQueue    *queue.Queue
	messageID    atomic.Uint64
	requestID    atomic.Uint64
	requestLock  sync.Mutex
	messageLock  sync.Mutex
	closeHandler func()
	messageTasks map[uint16]chan *packet.Messack
	requestTasks map[uint16]chan *packet.Response
}

func newConn(raw rawconn, logger *xlog.Logger, queueCapacity int32) Conn {
	c := &conn{
		raw:          raw,
		logger:       logger,
		sendQueue:    queue.New(int32(queueCapacity)),
		messageTasks: make(map[uint16]chan *packet.Messack),
		requestTasks: make(map[uint16]chan *packet.Response),
	}
	return c
}

func (c *conn) ID() *packet.Identity {
	return c.raw.ID()
}
func (c *conn) IP() string {
	return c.raw.IP()
}
func (c *conn) IsIdle() bool {
	return c.sendQueue.IsIdle()
}

func (c *conn) IsClosed() bool {
	return c.closed.Load()
}
func (c *conn) OnClose(handler func()) {
	c.closeHandler = handler
}
func (c *conn) SendPong() error {
	return c.jumpPacket(context.Background(), packet.NewPong())
}
func (c *conn) SendPing(ctx context.Context) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	c.pingLock.Lock()
	c.pingChan = make(chan struct{})
	c.pingLock.Unlock()
	defer func() {
		c.pingLock.Lock()
		close(c.pingChan)
		c.pingChan = nil
		c.pingLock.Unlock()
	}()
	if err := c.SendPacket(ctx, packet.NewPing()); err != nil {
		return err
	}
	select {
	case <-c.pingChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *conn) SendMessage(ctx context.Context, p *packet.Message) error {
	if p.Qos == packet.MessageQos0 {
		return c.SendPacket(ctx, p)
	}
	if p.ID == 0 {
		p.ID = uint16(c.messageID.Add(1) / math.MaxUint16)
	}
	return c.retryInflightMessage(ctx, p, 0)
}
func (c *conn) retryInflightMessage(ctx context.Context, p *packet.Message, attempts int) error {
	if attempts > 5 {
		return xerr.MessageTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	_, err := c.sendInflightMessage(ctx, p)
	if err != nil {
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			p.Dup = true
			return c.retryInflightMessage(context.Background(), p, attempts+1)
		}
		return err
	}
	return nil
}
func (c *conn) sendInflightMessage(ctx context.Context, p *packet.Message) (*packet.Messack, error) {
	if c.IsClosed() {
		return nil, xerr.ConnectionIsClosed
	}
	c.messageLock.Lock()
	ackCh := make(chan *packet.Messack)
	c.messageTasks[p.ID] = ackCh
	c.messageLock.Unlock()
	defer func() {
		c.messageLock.Lock()
		delete(c.messageTasks, p.ID)
		close(ackCh)
		c.messageLock.Unlock()
	}()
	if err := c.SendPacket(ctx, p); err != nil {
		return nil, err
	}
	select {
	case ack := <-ackCh:
		return ack, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if c.IsClosed() {
		return nil, xerr.ConnectionIsClosed
	}
	if p.ID == 0 {
		p.ID = uint16(c.requestID.Add(1) / math.MaxUint16)
	}
	c.requestLock.Lock()
	resp := make(chan *packet.Response)
	c.requestTasks[p.ID] = resp
	c.requestLock.Unlock()
	defer func() {
		c.requestLock.Lock()
		delete(c.requestTasks, p.ID)
		close(resp)
		c.requestLock.Unlock()
	}()
	if err := c.SendPacket(ctx, p); err != nil {
		return nil, err
	}
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *conn) RecvPong() {
	c.pingLock.Lock()
	if c.pingChan != nil {
		c.pingChan <- struct{}{}
	}
	c.pingLock.Unlock()
}
func (c *conn) RecvMessack(p *packet.Messack) {
	c.messageLock.Lock()
	ch, ok := c.messageTasks[p.ID()]
	if ok {
		ch <- p
	}
	c.messageLock.Unlock()
	if !ok {
		c.logger.Error("response task not found", xlog.U16("id", p.ID()))
	}
}
func (c *conn) RecvResponse(p *packet.Response) {
	c.requestLock.Lock()
	ch, ok := c.requestTasks[p.ID()]
	if ok {
		ch <- p
	}
	c.requestLock.Unlock()
	if !ok {
		c.logger.Error("response task not found", xlog.U16("id", p.ID()))
	}
}
func (c *conn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.sendQueue.Close()
		err := c.raw.Close()
		if c.closeHandler != nil {
			c.closeHandler()
		}
		return err
	}
	return nil
}
func (c *conn) CloseClode(code packet.CloseCode) error {
	c.jumpPacket(context.Background(), packet.NewClose(code))
	return c.Close()
}
func (c *conn) ConnackCode(code packet.ConnectCode, connId string) error {
	p := packet.NewConnack(code)
	if connId != "" {
		p.Set(packet.PropertyConnID, connId)
	}
	return c.jumpPacket(context.Background(), p)
}
func (c *conn) SendPacket(ctx context.Context, p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	return c.sendQueue.Push(ctx, false, func() {
		err := c.raw.WriteData(data)
		if err != nil {
			c.logger.Warn("write data error", xlog.Err(err))
		}
	})
}
func (c *conn) jumpPacket(ctx context.Context, p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	return c.sendQueue.Push(ctx, true, func() {
		err := c.raw.WriteData(data)
		if err != nil {
			c.logger.Warn("write data error", xlog.Err(err))
		}
	})
}
func genConnId(s string) string {
	h := crc32.NewIEEE()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
func getAddrIp(addr string) string {
	strs := strings.Split(addr, ":")
	return strs[0]
}
func getRealIP(r *http.Request) string {
	realIP := r.Header.Get("X-Real-IP")
	if realIP == "" {
		realIP = getAddrIp(r.RemoteAddr)
	}
	return realIP
}
func assertTLS(config *tls.Config) {
	if config == nil {
		panic("tls config is nil")
	}
	configHasCert := len(config.Certificates) > 0 || config.GetCertificate != nil || config.GetConfigForClient != nil
	if !configHasCert {
		panic("tls config has no certificate")
	}
}

type pinger struct {
	conn      Conn
	timeout   time.Duration
	keepalive *keepalive.KeepAlive
}

func newPinger(c Conn, interval, timeout time.Duration) *pinger {
	p := &pinger{
		conn:    c,
		timeout: timeout,
	}
	p.keepalive = keepalive.New(interval, p)
	return p
}
func (p *pinger) Start() {
	p.keepalive.Start()
}
func (p *pinger) Stop() {
	p.keepalive.Stop()
}
func (p *pinger) SendPing() {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	err := p.conn.SendPing(ctx)
	if err != nil {
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			p.conn.CloseClode(packet.ClosePingTimeOut)
		}
	}
}
