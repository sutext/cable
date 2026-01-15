package client

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Error uint8

const (
	ErrInvalidPacket      Error = 0
	ErrRequestTimeout     Error = 1
	ErrConnectionFailed   Error = 2
	ErrConnectionClosed   Error = 3
	ErrConnectionNotReady Error = 4
)

func (e Error) Error() string {
	switch e {
	case ErrInvalidPacket:
		return "invalid packet"
	case ErrRequestTimeout:
		return "request timeout"
	case ErrConnectionFailed:
		return "connection failed"
	case ErrConnectionClosed:
		return "connection closed"
	case ErrConnectionNotReady:
		return "connection not ready"
	default:
		return "unknown error"
	}
}

type Client interface {
	ID() *packet.Identity
	Status() Status
	Connect(identity *packet.Identity)
	IsReady() bool
	SendMessage(ctx context.Context, p *packet.Message) error
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
}
type client struct {
	id             *packet.Identity
	conn           Conn
	connId         string
	closed         atomic.Bool
	status         Status
	logger         *xlog.Logger
	retrier        *Retrier
	handler        Handler
	retrying       bool
	pingChan       chan struct{}
	pingLock       sync.Mutex
	sendQueue      *queue.Queue
	keepalive      *keepalive.KeepAlive
	statusLock     sync.RWMutex
	pingTimeout    time.Duration
	pingInterval   time.Duration
	requestLock    sync.Mutex
	messageLock    sync.Mutex
	requestTasks   map[int64]chan *packet.Response
	messageTasks   map[int64]chan *packet.Messack
	writeTimeout   time.Duration
	requestTimeout time.Duration
	messageTimeout time.Duration
}

func New(address string, options ...Option) Client {
	opts := newOptions(options...)
	var conn Conn
	switch opts.network {
	case NetworkTCP:
		conn = newTCPConn(address)
	case NetworkUDP:
		conn = newUDPConn(address)
	case NetworkQUIC:
		conn = newQUICConn(address, opts.quicConfig)
	case NetworkWS:
		conn = newWSConn(address)
	default:
		panic("unknown network")
	}
	c := &client{
		conn:           conn,
		status:         StatusUnknown,
		logger:         opts.logger,
		retrier:        opts.retrier,
		handler:        opts.handler,
		sendQueue:      queue.New(opts.sendQueueCapacity),
		pingTimeout:    opts.pingTimeout,
		pingInterval:   opts.pingInterval,
		writeTimeout:   opts.writeTimeout,
		requestTasks:   make(map[int64]chan *packet.Response),
		messageTasks:   make(map[int64]chan *packet.Messack),
		requestTimeout: opts.requestTimeout,
		messageTimeout: opts.messageTimeout,
	}
	c.keepalive = keepalive.New(c.pingInterval, c)
	return c
}
func (c *client) ID() *packet.Identity {
	return c.id
}
func (c *client) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.keepalive.Stop()
		c.sendQueue.Close()
		c.conn.Close()
	}
}

func (c *client) Connect(identity *packet.Identity) {
	c.id = identity
	switch c.Status() {
	case StatusOpened, StatusOpening:
		return
	}
	c._setStatus(StatusOpening)
	if err := c.reconnect(); err != nil {
		c.retrying = false
		c.retrier.Reset()
		c.tryClose(err)
	}
}
func (c *client) IsReady() bool {
	return c.Status() == StatusOpened
}
func (c *client) tryClose(err error) {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()
	if c.retrying {
		return
	}
	if c.id == nil {
		return
	}
	c.logger.Error("try close", xlog.Err(err))
	if c.status == StatusClosed || c.status == StatusClosing {
		return
	}
	if code, ok := err.(CloseReason); ok {
		if code == CloseReasonNormal {
			c._setStatus(StatusClosed)
			return
		}
	}
	if c.retrier == nil {
		c._setStatus(StatusClosed)
		return
	}
	delay, ok := c.retrier.Retry(err)
	if !ok {
		c._setStatus(StatusClosed)
		return
	}
	c.retrying = true
	c._setStatus(StatusOpening)
	time.AfterFunc(delay, func() {
		if c.Status() != StatusOpening {
			return
		}
		if err := c.reconnect(); err != nil {
			c.retrying = false
			c.tryClose(err)
		}
	})
}

func (c *client) reconnect() error {
	err := c.conn.Dail()
	if err != nil {
		return err
	}
	err = c.conn.WritePacket(packet.NewConnect(c.id))
	if err != nil {
		return err
	}
	p, err := c.conn.ReadPacket()
	if err != nil {
		return err
	}
	if p.Type() != packet.CONNACK {
		return ErrConnectionFailed
	}
	ack := p.(*packet.Connack)
	if ack.Code != packet.ConnectAccepted {
		return ack.Code
	}
	if connID, ok := ack.Get(packet.PropertyConnID); ok {
		c.connId = connID
	}
	c.setStatus(StatusOpened)
	return nil
}

func (c *client) SendPing() {
	ctx, cancel := context.WithTimeout(context.Background(), c.pingTimeout)
	defer cancel()
	err := c.sendPing(ctx)
	if err != nil {
		c.logger.Error("send ping error", xlog.Err(err))
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			c.tryClose(CloseReasonPingTimeout)
		}
	}
}
func (c *client) sendPing(ctx context.Context) error {
	if !c.IsReady() {
		return ErrConnectionNotReady
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
	if err := c.sendPacket(ctx, false, packet.NewPing()); err != nil {
		return err
	}
	select {
	case <-c.pingChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *client) sendPong() error {
	return c.sendPacket(context.Background(), true, packet.NewPong())
}
func (c *client) SendMessage(ctx context.Context, p *packet.Message) error {
	if !c.IsReady() {
		return ErrConnectionNotReady
	}
	if p.Qos == packet.MessageQos0 {
		return c.sendPacket(ctx, false, p)
	}
	return c.retryInflightMessage(ctx, p, 0)
}
func (c *client) retryInflightMessage(ctx context.Context, p *packet.Message, attempts int) error {
	if attempts > 5 {
		return ErrRequestTimeout
	}
	if attempts > 0 {
		p.Dup = true
		c.logger.Warn("retry inflight message", xlog.I64("id", p.ID()), xlog.Int("attempts", attempts))
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
func (c *client) sendInflightMessage(ctx context.Context, p *packet.Message) (*packet.Messack, error) {
	if !c.IsReady() {
		return nil, ErrConnectionNotReady
	}
	c.messageLock.Lock()
	ackCh := make(chan *packet.Messack)
	c.messageTasks[p.ID()] = ackCh
	c.messageLock.Unlock()
	defer func() {
		c.messageLock.Lock()
		delete(c.messageTasks, p.ID())
		close(ackCh)
		c.messageLock.Unlock()
	}()
	if err := c.sendPacket(ctx, false, p); err != nil {
		return nil, err
	}
	select {
	case res := <-ackCh:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *client) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if !c.IsReady() {
		return nil, ErrConnectionNotReady
	}
	c.requestLock.Lock()
	resp := make(chan *packet.Response)
	c.requestTasks[p.ID()] = resp
	c.requestLock.Unlock()
	defer func() {
		c.requestLock.Lock()
		delete(c.requestTasks, p.ID())
		close(resp)
		c.requestLock.Unlock()
	}()
	if err := c.sendPacket(ctx, false, p); err != nil {
		return nil, err
	}
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *client) sendPacket(ctx context.Context, jump bool, p packet.Packet) error {
	if !c.IsReady() {
		return ErrConnectionNotReady
	}
	return c.sendQueue.Push(ctx, jump, func() {
		if !c.IsReady() {
			return
		}
		if c.connId != "" {
			p.Set(packet.PropertyConnID, c.connId)
		}
		err := c.conn.WritePacket(p)
		if err != nil {
			c.tryClose(err)
			return
		}
		c.keepalive.UpdateTime()
	})
}
func (c *client) Status() Status {
	c.statusLock.RLock()
	defer c.statusLock.RUnlock()
	return c.status
}
func (c *client) setStatus(status Status) {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()
	c._setStatus(status)
}
func (c *client) _setStatus(status Status) {
	if c.status == status {
		return
	}
	c.logger.Debug("client status change", xlog.Str("from", c.status.String()), xlog.Str("to", status.String()))
	c.status = status
	switch status {
	case StatusClosed:
		c.conn.Close()
		c.keepalive.Stop()
	case StatusOpening, StatusClosing:
		c.keepalive.Stop()
	case StatusOpened:
		c.keepalive.Start()
		go c.recv()
	}
	go c.handler.OnStatus(status)
}

func (c *client) recv() {
	for {
		p, err := c.conn.ReadPacket()
		if err != nil {
			c.tryClose(err)
			return
		}
		go c.handlePacket(p)
	}
}

func (c *client) handlePacket(p packet.Packet) {
	c.keepalive.UpdateTime()
	switch p.Type() {
	case packet.MESSAGE:
		msg := p.(*packet.Message)
		err := c.handler.OnMessage(msg)
		if err != nil {
			c.logger.Error("message handler error", xlog.Err(err))
			return
		}
		if msg.Qos == packet.MessageQos1 {
			if err := c.sendPacket(context.Background(), true, msg.Ack()); err != nil {
				c.logger.Error("send messack packet error", xlog.Err(err))
			}
		}
	case packet.MESSACK:
		p := p.(*packet.Messack)
		c.messageLock.Lock()
		ch, ok := c.messageTasks[p.ID()]
		if ok {
			ch <- p
		}
		c.messageLock.Unlock()
		if !ok {
			c.logger.Error("response task not found", xlog.I64("id", p.ID()))
		}
	case packet.REQUEST:
		res, err := c.handler.OnRequest(p.(*packet.Request))
		if err != nil {
			c.logger.Error("request handler error", xlog.Err(err))
			return
		}
		if res != nil {
			if err := c.sendPacket(context.Background(), false, res); err != nil {
				c.logger.Error("send response packet error", xlog.Err(err))
			}
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		c.requestLock.Lock()
		ch, ok := c.requestTasks[p.ID()]
		if ok {
			ch <- p
		}
		c.requestLock.Unlock()
		if !ok {
			c.logger.Error("response task not found", xlog.I64("id", p.ID()))
		}
	case packet.PING:
		if err := c.sendPong(); err != nil {
			c.logger.Error("send pong error", xlog.Err(err))
		}
	case packet.PONG:
		c.pingLock.Lock()
		if c.pingChan != nil {
			c.pingChan <- struct{}{}
		}
		c.pingLock.Unlock()
	case packet.CLOSE:
		c.tryClose(p.(*packet.Close).Code)
	default:

	}
}
