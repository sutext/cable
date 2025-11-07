package client

import (
	"context"
	"sync"
	"time"

	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Error uint8

const (
	ErrNotConnected Error = iota
	ErrRequestTimeout
)

func (e Error) Error() string {
	return [...]string{"ErrNotConnected", "ErrRequestTimeout"}[e]
}

type Client interface {
	ID() *packet.Identity
	Status() Status
	Connect(identity *packet.Identity)
	Request(ctx context.Context, p *packet.Request) (*packet.Response, error)
	SendMessage(p *packet.Message) error
}
type client struct {
	mu             *sync.RWMutex
	conn           *conn
	tasks          sync.Map
	status         Status
	logger         logger.Logger
	address        string
	retrier        *Retrier
	handler        Handler
	identity       *packet.Identity
	retrying       bool
	keepalive      *keepalive.KeepAlive
	requestTimeout time.Duration
}

func New(address string, options ...Option) Client {
	opts := newOptions(options...)
	c := &client{
		mu:             new(sync.RWMutex),
		status:         StatusUnknown,
		logger:         opts.logger,
		address:        address,
		retrier:        NewRetrier(opts.retryLimit, opts.retryBackoff),
		handler:        opts.handler,
		keepalive:      keepalive.New(opts.pingInterval, opts.pingTimeout),
		requestTimeout: opts.requestTimeout,
	}
	c.keepalive.PingFunc(func() {
		c.sendPacket(packet.NewPing())
	})
	c.keepalive.TimeoutFunc(func() {
		c.tryClose(CloseReasonPingTimeout)
	})
	return c
}
func (c *client) ID() *packet.Identity {
	return c.identity
}
func (c *client) Connect(identity *packet.Identity) {
	c.identity = identity
	switch c.Status() {
	case StatusOpened, StatusOpening:
		return
	}
	c.setStatus(StatusOpening)
	c.reconnect()
}
func (c *client) tryClose(err error) {
	c.logger.Error("try close", "reason", err)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.identity == nil {
		return
	}
	if c.status == StatusClosed || c.status == StatusClosing {
		return
	}
	if c.retrying {
		return
	}
	if code, ok := err.(CloseReason); ok {
		if code == CloseReasonNormal {
			c.setStatus(StatusClosed)
			return
		}
	}
	if c.retrier == nil {
		c.setStatus(StatusClosed)
		return
	}
	delay, ok := c.retrier.can(err)
	if !ok {
		c.setStatus(StatusClosed)
		return
	}
	c.retrying = true
	c.setStatus(StatusOpening)
	c.logger.Info("will retry after", "delay", delay.String())
	c.retrier.retry(delay, func() {
		c.retrying = false
		c.reconnect()
	})

}
func (c *client) reconnect() {
	if c.conn != nil {
		c.conn.close()
		c.conn = nil
	}
	c.conn = &conn{}
	c.conn.onPacket(c.handlePacket)
	c.conn.onError(c.tryClose)
	err := c.conn.connect(c.address)
	if err != nil {
		c.tryClose(err)
		return
	}
	c.sendPacket(packet.NewConnect(c.identity))
}

func (c *client) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *client) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *client) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *client) Request(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	c.sendPacket(p)
	resp := make(chan *packet.Response)
	c.tasks.Store(p.Serial, resp)
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(c.requestTimeout):
		return nil, ErrRequestTimeout
	}
}
func (c *client) sendPacket(p packet.Packet) error {
	if c.conn == nil {
		return ErrNotConnected
	}
	return c.conn.sendPacket(p)
}
func (c *client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}
func (c *client) safeSetStatus(status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setStatus(status)
}
func (c *client) setStatus(status Status) {
	if c.status == status {
		return
	}
	c.logger.Info("status change", "from", c.status.String(), "to", status.String())
	c.status = status
	switch status {
	case StatusClosed:
		c.keepalive.Stop()
		c.retrier.cancel()
		if c.conn != nil {
			c.conn.close()
			c.conn = nil
		}
	case StatusOpening, StatusClosing:
		c.keepalive.Stop()
	case StatusOpened:
		c.keepalive.Start()
	}
}
func (c *client) handlePacket(p packet.Packet) {
	c.logger.Info("receive", "packet", p.String())
	switch p.Type() {
	case packet.CONNACK:
		p := p.(*packet.Connack)
		if p.Code != 0 {
			return
		}
		c.safeSetStatus(StatusOpened)
	case packet.MESSAGE:
		c.handler.OnMessage(p.(*packet.Message))
	case packet.REQUEST:
		res, err := c.handler.OnRequest(p.(*packet.Request))
		if err != nil {
			c.logger.Error("request handler error", "error", err)
		}
		if res != nil {
			c.sendPacket(res)
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		if t, ok := c.tasks.LoadAndDelete(p.Serial); ok {
			t.(chan *packet.Response) <- p
		} else {
			c.logger.Error("response task not found", "serial", p.Serial)
		}
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		c.keepalive.HandlePong()
	case packet.CLOSE:
		c.tryClose(p.(*packet.Close).Code)
	default:

	}
}
