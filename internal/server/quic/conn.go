package quic

import (
	"context"
	"fmt"
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
		c.Close(packet.CloseNormal)
	})
	return c
}
func (c *conn) GetID() *packet.Identity {
	return c.id
}

func (c *conn) Close(code packet.CloseCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.raw == nil {
		return
	}
	c.SendPacket(packet.NewClose(code))
	c.raw.Close()
	c.keepAlive.Stop()
	close(c.authed)
	c.clear()
}
func (c *conn) closed() bool {
	return c.raw == nil
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
		packet, err := packet.ReadFrom(c.stream)
		if err != nil {
			return
		}
		go c.handlePacket(packet)
	}
}

func (c *conn) connack(code packet.ConnectCode) error {
	return c.SendPacket(packet.NewConnack(code))
}
func (c *conn) SendPing() error {
	return c.SendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.SendPacket(packet.NewPong())
}
func (c *conn) SendData(data []byte) error {
	return c.SendPacket(packet.NewData(data))
}
func (c *conn) SendPacket(p packet.Packet) error {
	if c.stream == nil {
		return server.ErrConnectionClosed
	}
	return packet.WriteTo(c.stream, p)
}
func (c *conn) doAuth(id *packet.Identity) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed() {
		return server.ErrConnectionClosed
	}
	if c.id != nil {
		return fmt.Errorf("already login")
	}
	err := c.server.onAuth(id)
	if err != nil {
		return err
	}
	c.id = id
	c.authed <- struct{}{}
	go c.server.addConn(c)
	c.keepAlive.Start()
	return nil
}
func (c *conn) handlePacket(p packet.Packet) {
	if c.closed() {
		return
	}
	switch p.Type() {
	case packet.CONNECT:
		p := p.(*packet.ConnectPacket)
		if p.Identity != nil {
			err := c.doAuth(p.Identity)
			if err != nil {
				c.Close(packet.CloseAuthenticationFailure)
				return
			}
		}
		c.connack(packet.ConnectionAccepted)
	case packet.DATA:
		if c.id == nil {
			return
		}
		p := p.(*packet.DataPacket)
		res, err := c.server.onData(c.id, p)
		if err != nil {
			return
		}
		if res != nil {
			c.SendPacket(res)
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
