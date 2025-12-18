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
	ErrRequestTimeout     Error = 1
	ErrConnectionFailed   Error = 2
	ErrConnectionClosed   Error = 3
	ErrConnectionNotReady Error = 4
	ErrInvalidPacket      Error = 5
)

func (e Error) Error() string {
	switch e {
	case ErrRequestTimeout:
		return "request timeout"
	case ErrConnectionFailed:
		return "connection failed"
	case ErrConnectionClosed:
		return "connection closed"
	case ErrConnectionNotReady:
		return "connection not ready"
	case ErrInvalidPacket:
		return "invalid packet"
	default:
		return "unknown error"
	}
}

type Client interface {
	ID() *packet.Identity
	Status() Status
	Connect(identity *packet.Identity)
	IsReady() bool
	SendQueueLength() int32
	SendMessage(ctx context.Context, p *packet.Message) error
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
}
type client struct {
	id             *packet.Identity
	conn           Conn
	closed         atomic.Bool
	status         Status
	logger         *xlog.Logger
	retrier        *Retrier
	handler        Handler
	retrying       bool
	sendQueue      *queue.Queue
	keepalive      *keepalive.KeepAlive
	statusLock     sync.RWMutex
	pingTimeout    time.Duration
	pingInterval   time.Duration
	packetConnID   string
	requestTasks   sync.Map
	messageTasks   sync.Map
	writeTimeout   time.Duration
	requestTimeout time.Duration
	messageTimeout time.Duration
}

func New(address string, options ...Option) Client {
	opts := newOptions(options...)
	c := &client{
		conn:           NewConn(opts.transport, address),
		status:         StatusUnknown,
		logger:         opts.logger,
		retrier:        opts.retrier,
		handler:        opts.handler,
		sendQueue:      queue.New(opts.sendQueueCapacity),
		pingTimeout:    opts.pingTimeout,
		pingInterval:   opts.pingInterval,
		writeTimeout:   opts.writeTimeout,
		requestTimeout: opts.requestTimeout,
		messageTimeout: opts.messageTimeout,
	}
	c.keepalive = keepalive.New(c.pingInterval, c.pingTimeout)
	c.keepalive.PingFunc(func() error {
		if err := c.SendPing(); err != nil {
			c.logger.Error("send ping error", xlog.Err(err))
			return err
		}
		return nil
	})
	c.keepalive.TimeoutFunc(func() {
		c.tryClose(CloseReasonPingTimeout)
	})
	return c
}
func (c *client) ID() *packet.Identity {
	return c.id
}
func (c *client) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.keepalive.Close()
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
		c.packetConnID = connID
	}
	c.setStatus(StatusOpened)
	return nil
}
func (c *client) SendPing() error {
	return c.sendPacket(context.Background(), true, packet.NewPing())
}
func (c *client) SendPong() error {
	return c.sendPacket(context.Background(), true, packet.NewPong())
}
func (c *client) SendQueueLength() int32 {
	return c.sendQueue.Len()
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
	ackCh := make(chan *packet.Messack)
	defer close(ackCh)
	c.messageTasks.Store(p.ID, ackCh)
	defer c.messageTasks.Delete(p.ID)
	if err := c.sendPacket(ctx, false, p); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.messageTimeout)
	defer cancel()
	select {
	case res := <-ackCh:
		return res, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}
func (c *client) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if !c.IsReady() {
		return nil, ErrConnectionNotReady
	}
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.ID, resp)
	defer c.requestTasks.Delete(p.ID)
	if err := c.sendPacket(ctx, false, p); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
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
		if c.packetConnID != "" {
			p.Set(packet.PropertyConnID, c.packetConnID)
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
	case StatusOpening, StatusClosing:
		break
	case StatusOpened:
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
		if connID, ok := p.Get(packet.PropertyConnID); ok {
			if connID != c.packetConnID {
				c.logger.Error("packet conn id not match", xlog.Str("connID", connID), xlog.Str("expect", c.packetConnID))
				c.tryClose(ErrConnectionFailed)
				return
			}
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
			if err := c.sendPacket(context.Background(), true, packet.NewMessack(msg.ID)); err != nil {
				c.logger.Error("send messack packet error", xlog.Err(err))
			}
		}
	case packet.MESSACK:
		p := p.(*packet.Messack)
		if t, ok := c.messageTasks.Load(p.ID); ok {
			t.(chan *packet.Messack) <- p
		} else {
			c.logger.Error("response task not found", xlog.I64("id", p.ID))
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
		if t, ok := c.requestTasks.Load(p.ID); ok {
			t.(chan *packet.Response) <- p
		} else {
			c.logger.Error("response task not found", xlog.I64("id", p.ID))
		}
	case packet.PING:
		if err := c.SendPong(); err != nil {
			c.logger.Error("send pong error", xlog.Err(err))
		}
	case packet.PONG:
		c.keepalive.HandlePong()
	case packet.CLOSE:
		c.tryClose(p.(*packet.Close).Code)
	default:

	}
}
