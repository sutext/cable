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
	SendMessage(p *packet.Message) error
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
}
type client struct {
	id             *packet.Identity
	mu             *sync.RWMutex
	conn           *net.UDPConn
	status         Status
	logger         logger.Logger
	address        string
	retrier        *Retrier
	handler        Handler
	retrying       bool
	recvQueue      *queue.Queue
	sendQueue      *queue.Queue
	keepalive      *keepalive.KeepAlive
	requestTasks   sync.Map
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
		recvQueue:      queue.New(1024),
		sendQueue:      queue.New(1024),
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
	c.logger.Error("try close", "reason", err)
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

//	func (c *client) reconnect() error {
//		if c.conn != nil {
//			c.conn.Close()
//			c.conn = nil
//		}
//		conn, err := net.Dial("tcp", c.address)
//		if err != nil {
//			return err
//		}
//		c.conn = conn
//		if err := packet.WriteTo(conn, packet.NewConnect(c.id)); err != nil {
//			return err
//		}
//		p, err := packet.ReadFrom(c.conn)
//		if err != nil {
//			return err
//		}
//		if p.Type() != packet.CONNACK {
//			return ErrConntionFailed
//		}
//		if p.(*packet.Connack).Code != packet.ConnectionAccepted {
//			return ErrConntionFailed
//		}
//		c.setStatus(StatusOpened)
//		return nil
//	}
func (c *client) reconnect() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	c.conn = conn

	connData, err := packet.Marshal(packet.NewConnect(c.id))
	if err != nil {
		return err
	}
	addr, err := net.ResolveUDPAddr("udp4", c.address)
	if err != nil {
		return err
	}
	if _, err := conn.WriteToUDP(connData, addr); err != nil {
		return err
	}
	buf := make([]byte, 2048)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return err
	}
	p, err := packet.Unmarshal(buf[:n])
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

//	func (c *client) sendPacket(p packet.Packet) error {
//		return c.sendQueue.AddTask(func() {
//			if c.Status() != StatusOpened {
//				return
//			}
//			if err := packet.WriteTo(c.conn, p); err != nil {
//				c.tryClose(err)
//				return
//			}
//			c.keepalive.UpdateTime()
//		})
//	}
func (c *client) sendPacket(p packet.Packet) error {
	return c.sendQueue.AddTask(func() {
		if c.Status() != StatusOpened {
			return
		}
		data, err := packet.Marshal(p)
		if err != nil {
			c.tryClose(err)
			return
		}
		addr, err := net.ResolveUDPAddr("udp4", c.address)
		if err != nil {
			c.tryClose(err)
			return
		}
		_, err = c.conn.WriteToUDP(data, addr)
		if err != nil {
			c.tryClose(err)
			return
		}
		c.logger.Debug("send packet", "packet", p)
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
	c.logger.Debug("status change", "from", c.status.String(), "to", status.String())
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
		go c.recv()
	}
}

//	func (c *client) recv() {
//		for {
//			p, err := packet.ReadFrom(c.conn)
//			if err != nil {
//				c.tryClose(err)
//				return
//			}
//			c.recvQueue.AddTask(func() {
//				c.handlePacket(p)
//				c.keepalive.UpdateTime()
//			})
//		}
//	}
func (c *client) recv() {
	for {
		buf := make([]byte, 2048)
		n, _, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			c.tryClose(err)
			return
		}
		p, err := packet.Unmarshal(buf[:n])
		if err != nil {
			c.tryClose(err)
			return
		}
		c.recvQueue.AddTask(func() {
			c.handlePacket(p)
			c.keepalive.UpdateTime()
		})
	}
}
func (c *client) handlePacket(p packet.Packet) {
	c.logger.Debug("receive packet", "packet", p)
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
		if t, ok := c.requestTasks.Load(p.ID); ok {
			t.(chan *packet.Response) <- p
		} else {
			c.logger.Error("response task not found", "seq", p.ID)
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
