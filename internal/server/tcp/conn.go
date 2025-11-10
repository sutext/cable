package tcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type conn struct {
	id     *packet.Identity
	mu     *sync.RWMutex
	raw    net.Conn
	tasks  sync.Map
	server *tcpServer
	authed chan struct{}
	logger logger.Logger
	// keepAlive *keepalive.KeepAlive
}

func newConn(raw net.Conn, server *tcpServer) *conn {
	c := &conn{
		mu:     new(sync.RWMutex),
		raw:    raw,
		server: server,
		logger: server.logger,
		authed: make(chan struct{}),
	}
	// c.keepAlive = keepalive.New(time.Second*60, time.Second*5)
	// c.keepAlive.PingFunc(func() {
	// 	c.SendPing()
	// })
	// c.keepAlive.TimeoutFunc(func() {
	// 	c.Close(packet.CloseNoHeartbeat)
	// })
	return c
}
func (c *conn) ID() *packet.Identity {
	return c.id
}
func (c *conn) IsActive() bool {
	return c.raw != nil
}
func (c *conn) Close(code packet.CloseCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.raw == nil {
		return
	}
	c.sendPacket(packet.NewClose(code))
	c.raw.Close()
	// c.keepAlive.Stop()
	close(c.authed)
	c.clear()
}

func (c *conn) clear() {
	c.server.delConn(c)
	c.mu = nil
	c.raw = nil
	// c.keepAlive = nil
	c.server = nil
	c.authed = nil
}
func (c *conn) serve() {
	go func() {
		timer := time.NewTimer(time.Second * 10)
		defer timer.Stop()
		select {
		case <-c.authed:
			return
		case <-timer.C:
			c.Close(packet.CloseAuthenticationTimeout)
			return
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
	c.tasks.Store(p.Seq, resp)
	c.sendPacket(p)
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, errors.New("request timeout")
	}
}

func (c *conn) sendPacket(p packet.Packet) error {
	if c.raw == nil {
		return server.ErrConnectionClosed
	}
	return packet.WriteTo(c.raw, p)
}
func (c *conn) doAuth(id *packet.Connect) packet.ConnackCode {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id != nil {
		return packet.ConnectionAccepted
	}
	code := c.server.connectHander(c.server, id)
	if code == packet.ConnectionAccepted {
		c.id = id.Identity
		c.authed <- struct{}{}
		go c.server.addConn(c)
		// c.keepAlive.Start()
	}

	return code
}
func (c *conn) handlePacket(p packet.Packet) {
	if !c.IsActive() {
		c.logger.Debug("connection closed when handling packet", "packetType", p.Type())
		return
	}
	switch p.Type() {
	case packet.CONNECT:
		p := p.(*packet.Connect)
		c.sendPacket(packet.NewConnack(c.doAuth(p)))
	case packet.MESSAGE:
		if c.id != nil {
			c.server.messageHandler(c.server, p.(*packet.Message), c.id)
		}

	case packet.REQUEST:
		if c.id == nil {
			return
		}
		p := p.(*packet.Request)
		res, err := c.server.requestHandler(c.server, p, c.id)
		if err != nil {
			return
		}
		if res != nil {
			c.sendPacket(res)
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		if resp, ok := c.tasks.LoadAndDelete(p.Seq); ok {
			resp.(chan *packet.Response) <- p
		}
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		// c.keepAlive.HandlePong()
		break
	case packet.CLOSE:
		c.clear()
	default:
		break
	}
}
