package tcp

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type conn struct {
	id           *packet.Identity
	raw          net.Conn
	closed       atomic.Bool
	authed       atomic.Bool
	server       *tcpServer
	logger       logger.Logger
	authChan     chan struct{}
	sendQueue    chan packet.Packet
	requestTasks sync.Map
}

func newConn(raw net.Conn, server *tcpServer) *conn {
	c := &conn{
		raw:       raw,
		server:    server,
		logger:    server.logger,
		authChan:  make(chan struct{}),
		sendQueue: make(chan packet.Packet, 1024),
	}
	return c
}
func (c *conn) ID() *packet.Identity {
	return c.id
}
func (c *conn) IsActive() bool {
	return !c.closed.Load()
}
func (c *conn) Close(code packet.CloseCode) {
	c.sendPacket(packet.NewClose(code))
	c.close()
}

func (c *conn) close() {
	if c.closed.CompareAndSwap(false, true) {
		c.server.delConn(c)
		close(c.sendQueue)
		c.raw.Close()
		c.server = nil
		c.sendQueue = nil
	}
}
func (c *conn) setAuthed(id *packet.Identity) {
	if c.authed.CompareAndSwap(false, true) {
		c.id = id
		c.authChan <- struct{}{}
		close(c.authChan)
		c.authChan = nil
	}
}
func (c *conn) serve() {
	go func() {
		timer := time.NewTimer(time.Second * 10)
		defer timer.Stop()
		select {
		case <-c.authChan:
			return
		case <-timer.C:
			c.Close(packet.CloseAuthenticationTimeout)
			return
		}
	}()
	go func() {
		for data := range c.sendQueue {
			if err := packet.WriteTo(c.raw, data); err != nil {
				c.logger.Debug("failed to send packet", "error", err)
				switch err.(type) {
				case net.Error:
					c.close()
					return
				}
			}
		}
	}()
	for {
		p, err := packet.ReadFrom(c.raw)
		if err != nil {
			switch err.(type) {
			case packet.Error:
				c.Close(packet.CloseInvalidPacket)
			default:
				c.Close(packet.CloseInternalError)
			}
			return
		}
		go c.handlePacket(p)
	}
}
func (c *conn) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *conn) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *conn) Request(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.Seq, resp)
	defer c.requestTasks.Delete(p.Seq)
	if err := c.sendPacket(p); err != nil {
		return nil, err
	}
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, server.ErrRequestTimeout
	}
}

func (c *conn) sendPacket(p packet.Packet) error {
	select {
	case c.sendQueue <- p:
		return nil
	default:
		return server.ErrSendingQueueFull
	}
}

func (c *conn) handlePacket(p packet.Packet) {
	switch p.Type() {
	case packet.CONNECT:
		p := p.(*packet.Connect)
		code := c.server.connectHander(c.server, p)
		if code == packet.ConnectionAccepted {
			c.setAuthed(c.id)
			go c.server.addConn(c)
		}
		c.sendPacket(packet.NewConnack(code))
	case packet.MESSAGE:
		c.server.messageHandler(c.server, p.(*packet.Message), c.id)
	case packet.REQUEST:
		p := p.(*packet.Request)
		res, err := c.server.requestHandler(c.server, p, c.id)
		if err != nil {
			return
		}
		if err := c.sendPacket(res); err != nil {
			c.logger.Debug("failed to send response", "error", err)
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		if resp, ok := c.requestTasks.Load(p.Seq); ok {
			resp.(chan *packet.Response) <- p
		}
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		break
	case packet.CLOSE:
		c.close()
	default:
		break
	}
}
