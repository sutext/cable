package client

import (
	"context"
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
)

type Error uint8

const (
	ErrNotConnected Error = iota
	ErrRequestTimeout
	ErrConntionFailed
)

func (e Error) Error() string {
	return [...]string{"ErrNotConnected", "ErrRequestTimeout", "ErrConntionFailed"}[e]
}

type Client interface {
	ID() *packet.Identity
	Status() Status
	Connect(identity *packet.Identity)
	Request(ctx context.Context, p *packet.Request) (*packet.Response, error)
	SendMessage(p *packet.Message) error
}
type client struct {
	id             *packet.Identity
	mu             *sync.RWMutex
	conn           net.Conn
	tasks          sync.Map
	status         Status
	logger         logger.Logger
	address        string
	retrier        *Retrier
	handler        Handler
	retrying       bool
	recvQueue      *queue.Queue
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
		recvQueue:      queue.New(128, 4),
		keepalive:      keepalive.New(opts.pingInterval, opts.pingTimeout),
		requestTimeout: opts.requestTimeout,
	}
	c.keepalive.PingFunc(func() {
		if err := c.sendPacket(packet.NewPing()); err != nil {
			c.logger.Error("send ping error", "error", err)
		}
	})
	c.keepalive.TimeoutFunc(func() {
		c.tryClose(CloseReasonPingTimeout)
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
	c.setStatus(StatusOpening)
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
	c.logger.Error("try close", "reason", err)
	if c.status == StatusClosed || c.status == StatusClosing {
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
	c.retrier.retry(delay, func() {
		err := c.reconnect()
		c.retrying = false
		if err != nil {
			c.tryClose(err)
		}
	})
}
func (c *client) reconnect() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	conn, err := net.Dial("tcp", c.address)
	if err != nil {
		return err
	}
	c.conn = conn
	if err := c.sendPacket(packet.NewConnect(c.id)); err != nil {
		return err
	}
	p, err := packet.ReadFrom(c.conn)
	if err != nil {
		return err
	}
	if p.Type() != packet.CONNACK {
		return ErrConntionFailed
	}
	if p.(*packet.Connack).Code != packet.ConnectionAccepted {
		return ErrConntionFailed
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
func (c *client) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *client) Request(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.tasks.Store(p.Seq, resp)
	if err := c.sendPacket(p); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *client) sendPacket(p packet.Packet) error {
	if c.conn == nil {
		return ErrNotConnected
	}
	if p.Type() != packet.CONNECT && c.status != StatusOpened {
		return ErrNotConnected
	}
	return packet.WriteTo(c.conn, p)
}
func (c *client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
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
			c.conn.Close()
			c.conn = nil
		}
	case StatusOpening, StatusClosing:
		c.keepalive.Stop()
	case StatusOpened:
		c.keepalive.Start()
		go c.receive()
	}
}
func (c *client) receive() {
	for {
		p, err := packet.ReadFrom(c.conn)
		if err != nil {
			c.tryClose(err)
			return
		}
		c.recvQueue.Push(func() {
			c.handlePacket(p)
		})
	}
}
func (c *client) handlePacket(p packet.Packet) {
	switch p.Type() {
	case packet.MESSAGE:
		c.logger.Debug("receive message", "message", p.(*packet.Message))
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
		if t, ok := c.tasks.LoadAndDelete(p.Seq); ok {
			t.(chan *packet.Response) <- p
		} else {
			c.logger.Error("response task not found", "seq", p.Seq)
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
