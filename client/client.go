package client

import (
	"context"
	"sync"
	"time"

	"sutext.github.io/cable/internal/inflight"
	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Error uint8

const (
	ErrRequestTimeout Error = 1
	ErrConntionFailed Error = 2
)

func (e Error) Error() string {
	return [...]string{"ErrRequestTimeout", "ErrConntionFailed"}[e]
}

type Client interface {
	ID() *packet.Identity
	Status() Status
	Connect(identity *packet.Identity)
	SendMessage(ctx context.Context, p *packet.Message) error
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
}
type client struct {
	id             *packet.Identity
	mu             *sync.RWMutex
	conn           Conn
	status         Status
	logger         *xlog.Logger
	retrier        *Retrier
	handler        Handler
	retrying       bool
	recvQueue      *queue.Queue
	sendQueue      *queue.Queue
	keepalive      *keepalive.KeepAlive
	inflights      *inflight.Inflight
	packetConnID   string
	requestTasks   sync.Map
	requestTimeout time.Duration
}

func New(address string, options ...Option) Client {
	opts := newOptions(options...)
	c := &client{
		mu:             new(sync.RWMutex),
		conn:           NewConn(opts.network, address),
		status:         StatusUnknown,
		logger:         xlog.With("GROUP", "CLIENT"),
		retrier:        opts.retrier,
		handler:        opts.handler,
		recvQueue:      queue.New(1024),
		sendQueue:      queue.New(1024),
		keepalive:      keepalive.New(opts.pingInterval, opts.pingTimeout),
		requestTimeout: opts.requestTimeout,
	}
	c.keepalive.PingFunc(func() {
		if err := c.sendPacket(packet.NewPing()); err != nil {
			c.logger.Error("send ping error", xlog.Err(err))
		}
	})
	c.keepalive.TimeoutFunc(func() {
		c.tryClose(CloseReasonPingTimeout)
	})
	c.inflights = inflight.New(func(m *packet.Message) {
		c.sendPacket(m)
	})
	return c
}
func (c *client) ID() *packet.Identity {
	return c.id
}
func (c *client) Connect(identity *packet.Identity) {
	c.id = identity
	switch c.Status() {
	case StatusOpened, StatusOpening:
		return
	}
	c._setStatus(StatusOpening)
	if err := c.reconnect(); err != nil {
		c.tryClose(err)
	}
}
func (c *client) tryClose(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	delay, ok := c.retrier.can(err)
	if !ok {
		c._setStatus(StatusClosed)
		return
	}
	c.retrying = true
	c._setStatus(StatusOpening)
	c.retrier.retry(delay, func() {
		err := c.reconnect()
		c.retrying = false
		if err != nil {
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
		return ErrConntionFailed
	}
	ack := p.(*packet.Connack)
	if ack.Code != packet.ConnectAccepted {
		return ErrConntionFailed
	}
	if connID, ok := ack.Get(packet.PropertyConnID); ok {
		c.packetConnID = connID
	}
	c.setStatus(StatusOpened)
	return nil
}
func (c *client) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *client) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *client) SendMessage(ctx context.Context, p *packet.Message) error {
	if p.Qos == packet.MessageQos1 {
		c.inflights.Add(p)
	}
	return c.sendPacket(p)
}

func (c *client) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.ID, resp)
	defer c.requestTasks.Delete(p.ID)
	if err := c.sendPacket(p); err != nil {
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

func (c *client) sendPacket(p packet.Packet) error {
	return c.sendQueue.AddTask(func() {
		if c.Status() != StatusOpened {
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
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}
func (c *client) setStatus(status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c._setStatus(status)
}
func (c *client) _setStatus(status Status) {
	if c.status == status {
		return
	}
	c.logger.Debug("client status change", xlog.String("from", c.status.String()), xlog.String("to", status.String()))
	c.status = status
	switch status {
	case StatusClosed:
		c.keepalive.Stop()
		c.inflights.Stop()
		if c.retrier != nil {
			c.retrier.cancel()
		}
		c.conn.Close()
	case StatusOpening, StatusClosing:
		c.keepalive.Stop()
		c.inflights.Stop()
	case StatusOpened:
		c.keepalive.Start()
		c.inflights.Start()
		go c.recv()
	}
	c.handler.OnStatus(status)
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
				c.logger.Error("packet conn id not match", xlog.String("connID", connID), xlog.String("expect", c.packetConnID))
				c.tryClose(ErrConntionFailed)
				return
			}
		}
		c.recvQueue.AddTask(func() {
			c.handlePacket(p)
			c.keepalive.UpdateTime()
		})
	}
}

func (c *client) handlePacket(p packet.Packet) {
	switch p.Type() {
	case packet.MESSAGE:
		msg := p.(*packet.Message)
		err := c.handler.OnMessage(msg)
		if err != nil {
			c.logger.Error("message handler error", xlog.Err(err))
			return
		}
		if msg.Qos == packet.MessageQos1 {
			if err := c.sendPacket(packet.NewMessack(msg.ID)); err != nil {
				c.logger.Error("send messack packet error", xlog.Err(err))
			}
		}
	case packet.MESSACK:
		ack := p.(*packet.Messack)
		c.inflights.Remove(ack.ID)
	case packet.REQUEST:
		res, err := c.handler.OnRequest(p.(*packet.Request))
		if err != nil {
			c.logger.Error("request handler error", xlog.Err(err))
			return
		}
		if res != nil {
			if err := c.sendPacket(res); err != nil {
				c.logger.Error("send response packet error", xlog.Err(err))
			}
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		if t, ok := c.requestTasks.Load(p.ID); ok {
			t.(chan *packet.Response) <- p
		} else {
			c.logger.Error("response task not found", xlog.Int64("id", p.ID))
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
