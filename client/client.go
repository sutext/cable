package client

import (
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/internal/backoff"
	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

var ErrNotConnected = errors.New("not connected")

type OnData func(p *packet.MessagePacket) error
type Client struct {
	mu        *sync.RWMutex
	conn      *conn
	host      string
	port      string
	onData    OnData
	status    Status
	logger    logger.Logger
	retrier   *Retrier
	identity  *packet.Identity
	retrying  bool
	keepalive *keepalive.KeepAlive
}

func New(host, port string) *Client {
	c := &Client{
		mu:        new(sync.RWMutex),
		host:      host,
		port:      port,
		status:    StatusUnknown,
		logger:    logger.NewText(slog.LevelDebug),
		retrier:   NewRetrier(100000, backoff.Constant(time.Second*2)),
		keepalive: keepalive.New(60, 5),
	}
	c.keepalive.PingFunc(func() {
		c.SendPacket(packet.NewPing())
	})
	c.keepalive.TimeoutFunc(func() {
		c.tryClose(CloseReasonPingTimeout)
	})
	return c
}
func (c *Client) OnData(f OnData) {
	c.onData = f
}
func (c *Client) SetLogger(l logger.Logger) {
	c.logger = l
}
func (c *Client) SetRetrier(limit int, backoff backoff.Backoff) {
	c.retrier = NewRetrier(limit, backoff)
}
func (c *Client) SetKeepAlive(interval time.Duration, timeout time.Duration) {
	c.keepalive = keepalive.New(interval, timeout)
}
func (c *Client) Connect(identity *packet.Identity) {
	c.identity = identity
	switch c.Status() {
	case StatusOpened, StatusOpening:
		return
	}
	c.setStatus(StatusOpening)
	c.reconnect()
}
func (c *Client) tryClose(err error) {
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
func (c *Client) reconnect() {
	if c.conn != nil {
		c.conn.close()
		c.conn = nil
	}
	c.conn = &conn{}
	c.conn.onPacket(c.handlePacket)
	c.conn.onError(c.tryClose)
	err := c.conn.connect(net.JoinHostPort(c.host, c.port))
	if err != nil {
		c.tryClose(err)
		return
	}
	c.SendPacket(packet.NewConnect(c.identity))
}

func (c *Client) SendData(data []byte) error {
	return c.SendPacket(packet.NewMessage(data))
}

func (c *Client) SendPing() error {
	return c.SendPacket(packet.NewPing())
}
func (c *Client) SendPong() error {
	return c.SendPacket(packet.NewPong())
}
func (c *Client) SendPacket(p packet.Packet) error {
	if c.conn == nil {
		return ErrNotConnected
	}
	return c.conn.sendPacket(p)
}
func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}
func (c *Client) safeSetStatus(status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setStatus(status)
}
func (c *Client) setStatus(status Status) {
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
func (c *Client) handlePacket(p packet.Packet) {
	c.logger.Info("receive", "packet", p.String())
	switch p.Type() {
	case packet.CONNACK:
		p := p.(*packet.ConnackPacket)
		if p.Code != 0 {
			return
		}
		c.safeSetStatus(StatusOpened)
	case packet.MESSAGE:
		p := p.(*packet.MessagePacket)
		if c.onData != nil {
			err := c.onData(p)
			if err != nil {
				c.logger.Error("data handler error", "error", err)
			}
		}
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		c.keepalive.HandlePong()
	case packet.CLOSE:
		c.tryClose(p.(*packet.ClosePacket).Code)
	default:

	}
}
