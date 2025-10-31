package nio

import (
	"github.com/cloudwego/netpoll"
	"sutext.github.io/cable/packet"
)

type conn struct {
	netpoll.Connection
	id *packet.Identity
}

func (c *conn) GetID() *packet.Identity {
	return c.id
}
func (c *conn) Close(code packet.CloseCode) {
	c.sendPacket(packet.NewClose(code))
	c.Connection.Close()
}
func (c *conn) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *conn) SendData(data []byte) error {
	return c.sendPacket(packet.NewData(data))
}

func (c *conn) sendPacket(p packet.Packet) error {
	return packet.WriteTo(c.Connection, p)
}
