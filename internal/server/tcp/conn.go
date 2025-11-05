package tcp

import (
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/internal/buffer"
	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type conn struct {
	id        *packet.Identity
	mu        *sync.RWMutex
	raw       net.Conn
	server    *tcpServer
	authed    chan struct{}
	logger    logger.Logger
	keepAlive *keepalive.KeepAlive
}

func newConn(raw net.Conn, server *tcpServer) *conn {
	c := &conn{
		mu:     new(sync.RWMutex),
		raw:    raw,
		server: server,
		logger: server.logger,
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
	for {
		p, err := packet.ReadFrom(c.raw)
		if err != nil {
			switch err.(type) {
			case packet.Error, buffer.Error:
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
	return c.SendPacket(packet.NewConnack(code))
}
func (c *conn) SendPing() error {
	return c.SendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.SendPacket(packet.NewPong())
}
func (c *conn) SendData(data []byte) error {
	return c.SendPacket(packet.NewMessage(data))
}
func (c *conn) SendPacket(p packet.Packet) error {
	if c.raw == nil {
		return server.ErrConnectionClosed
	}
	return packet.WriteTo(c.raw, p)
}
func (c *conn) doAuth(id *packet.ConnectPacket) packet.ConnackCode {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id != nil {
		return packet.ConnectionAccepted
	}
	code := c.server.connectHandler(id)
	if code == packet.ConnectionAccepted {
		c.id = id.Identity
		c.authed <- struct{}{}
		go c.server.addConn(c)
		c.keepAlive.Start()
	}

	return code
}
func (c *conn) handlePacket(p packet.Packet) {
	if c.closed() {
		return
	}
	switch p.Type() {
	case packet.CONNECT:
		p := p.(*packet.ConnectPacket)
		c.connack(c.doAuth(p))
	case packet.MESSAGE:
		if c.id == nil {
			return
		}
		p := p.(*packet.MessagePacket)
		res, err := c.server.messageHandler(p, c.id)
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
