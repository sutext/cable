package client

import (
	"net"

	"sutext.github.io/cable/packet"
)

type conn struct {
	raw     net.Conn
	pkgFunc func(packet.Packet)
	errFunc func(error)
}

func (c *conn) onError(f func(error)) {
	c.errFunc = f
}
func (c *conn) onPacket(f func(packet.Packet)) {
	c.pkgFunc = f
}
func (c *conn) connect(address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	c.raw = conn
	go c.receive()
	return nil
}
func (c *conn) receive() {
	for {
		p, err := packet.ReadFrom(c.raw)
		if err != nil {
			if c.errFunc != nil {
				c.errFunc(err)
			}
			return
		}
		if c.pkgFunc != nil {
			c.pkgFunc(p)
		}
	}
}
func (c *conn) close() error {
	if c.raw == nil {
		return nil
	}
	err := c.raw.Close()
	c.pkgFunc = nil
	c.errFunc = nil
	return err
}

func (c *conn) sendPacket(p packet.Packet) error {
	if c.raw == nil {
		return ErrNotConnected
	}
	return packet.WriteTo(c.raw, p)
}
