package quic

import (
	"context"
	"errors"
	"sync"
	"time"

	qc "golang.org/x/net/quic"
	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type conn struct {
	id        *packet.Identity
	mu        *sync.RWMutex
	raw       *qc.Conn
	tasks     sync.Map
	stream    *qc.Stream
	server    *quicServer
	authed    chan struct{}
	keepAlive *keepalive.KeepAlive
}

func newConn(raw *qc.Conn, server *quicServer) *conn {
	c := &conn{
		mu:     new(sync.RWMutex),
		raw:    raw,
		server: server,
		authed: make(chan struct{}),
	}
	c.keepAlive = keepalive.New(time.Second*60, time.Second*5)
	c.keepAlive.PingFunc(func() {
		c.SendPing()
	})
	c.keepAlive.TimeoutFunc(func() {
		c.Close(packet.ClosePintTimeOut)
	})
	return c
}
func (c *conn) ID() *packet.Identity {
	return c.id
}

func (c *conn) Close(code packet.CloseCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.raw == nil {
		return
	}
	c.sendPacket(packet.NewClose(code))
	c.raw.Close()
	c.keepAlive.Stop()
	close(c.authed)
	c.clear()
}
func (c *conn) IsActive() bool {
	return c.raw != nil
}
func (c *conn) clear() {
	c.server.delConn(c)
	c.mu = nil
	c.raw = nil
	c.keepAlive = nil
	c.server = nil
	c.id = nil
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
	s, err := c.raw.AcceptStream(context.Background())
	if err != nil {
		return
	}
	c.stream = s
	for {
		p, err := packet.ReadFrom(c.stream)
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

func (c *conn) connack(code packet.ConnackCode) error {
	return c.sendPacket(packet.NewConnack(code))
}
func (c *conn) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *conn) SendData(data []byte) error {
	return c.sendPacket(packet.NewMessage(data))
}
func (c *conn) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *conn) Request(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	c.sendPacket(p)
	resp := make(chan *packet.Response)
	c.tasks.Store(p.Seq, resp)
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, errors.New("timeout")
	}
}
func (c *conn) sendPacket(p packet.Packet) error {
	if c.stream == nil {
		return server.ErrConnectionClosed
	}
	return packet.WriteTo(c.stream, p)
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
		c.keepAlive.Start()
	}

	return code
}
func (c *conn) handlePacket(p packet.Packet) {
	if !c.IsActive() {
		return
	}
	switch p.Type() {
	case packet.CONNECT:
		p := p.(*packet.Connect)
		c.connack(c.doAuth(p))
	case packet.MESSAGE:
		if c.id == nil {
			return
		}
		p := p.(*packet.Message)
		c.server.messageHandler(c.server, p, c.id)

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
			(resp.(chan *packet.Response)) <- p
		}
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		c.keepAlive.HandlePong()
	case packet.CLOSE:
		c.clear()
	default:
		break
	}
}
