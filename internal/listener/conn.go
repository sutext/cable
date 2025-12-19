package listener

import (
	"context"
	"encoding/hex"
	"hash/crc32"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type conn interface {
	id() *packet.Identity
	close() error
	writeData(data []byte) error
}
type Listener interface {
	Close(ctx context.Context) error
	Listen(addr string) error
	OnClose(handler func(c *Conn))
	OnPacket(handler func(p packet.Packet, c *Conn))
	OnAccept(handler func(p *packet.Connect, c *Conn) packet.ConnectCode)
}

type Conn struct {
	raw          conn
	logger       *xlog.Logger
	closed       atomic.Bool
	sendQueue    *queue.Queue
	pingLock     sync.Mutex
	pingChan     chan struct{}
	requestLock  sync.Mutex
	messageLock  sync.Mutex
	closeHandler func()
	requestTasks map[int64]chan *packet.Response
	messageTasks map[int64]chan *packet.Messack
}

func newConn(raw conn, logger *xlog.Logger, queueCapacity int32) *Conn {
	c := &Conn{
		raw:       raw,
		logger:    logger,
		sendQueue: queue.New(int32(queueCapacity)),
	}
	return c
}

func (c *Conn) ID() *packet.Identity {
	return c.raw.id()
}

func (c *Conn) IsIdle() bool {
	return c.sendQueue.IsIdle()
}

func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}
func (c *Conn) SendPong() error {
	return c.jumpPacket(context.Background(), packet.NewPong())
}
func (c *Conn) SendPing(ctx context.Context) error {
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
func (c *Conn) SendMessage(ctx context.Context, p *packet.Message) error {
	if p.Qos == packet.MessageQos0 {
		return c.SendPacket(ctx, p)
	}
	return c.retryInflightMessage(ctx, p, 0)
}
func (c *Conn) retryInflightMessage(ctx context.Context, p *packet.Message, attempts int) error {
	if attempts > 5 {
		return xerr.MessageTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	_, err := c.sendInflightMessage(ctx, p)
	if err != nil {
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			return c.retryInflightMessage(context.Background(), p, attempts+1)
		}
		return err
	}
	return nil
}
func (c *Conn) sendInflightMessage(ctx context.Context, p *packet.Message) (*packet.Messack, error) {
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
func (c *Conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if c.IsClosed() {
		return nil, xerr.ConnectionIsClosed
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
func (c *Conn) RecvPong() {
	c.pingLock.Lock()
	if c.pingChan != nil {
		c.pingChan <- struct{}{}
	}
	c.pingLock.Unlock()
}
func (c *Conn) RecvMessack(p *packet.Messack) {
	c.messageLock.Lock()
	ch, ok := c.messageTasks[p.ID]
	if ok {
		ch <- p
	}
	c.messageLock.Unlock()
	if !ok {
		c.logger.Error("response task not found", xlog.I64("id", p.ID))
	}
}
func (c *Conn) RecvResponse(p *packet.Response) {
	c.requestLock.Lock()
	ch, ok := c.requestTasks[p.ID]
	if ok {
		ch <- p
	}
	c.requestLock.Unlock()
	if !ok {
		c.logger.Error("response task not found", xlog.I64("id", p.ID))
	}
}
func (c *Conn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.sendQueue.Close()
		err := c.raw.close()
		if c.closeHandler != nil {
			c.closeHandler()
		}
		return err
	}
	return nil
}
func (c *Conn) CloseClode(code packet.CloseCode) error {
	c.jumpPacket(context.Background(), packet.NewClose(code))
	return c.Close()
}
func (c *Conn) ConnackCode(code packet.ConnectCode, connId string) error {
	p := packet.NewConnack(code)
	if connId != "" {
		p.Set(packet.PropertyConnID, connId)
	}
	return c.jumpPacket(context.Background(), p)
}
func (c *Conn) SendPacket(ctx context.Context, p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	return c.sendQueue.Push(ctx, false, func() {
		err := c.raw.writeData(data)
		if err != nil {
			c.logger.Error("write data error", xlog.Err(err))
		}
	})
}
func (c *Conn) jumpPacket(ctx context.Context, p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	return c.sendQueue.Push(ctx, true, func() {
		err := c.raw.writeData(data)
		if err != nil {
			c.logger.Error("write data error", xlog.Err(err))
		}
	})
}
func genConnId(s string) string {
	h := crc32.NewIEEE()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

type pinger struct {
	conn      *Conn
	timeout   time.Duration
	keepalive *keepalive.KeepAlive
}

func newPinger(conn *Conn, interval, timeout time.Duration) *pinger {
	p := &pinger{
		conn:    conn,
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
