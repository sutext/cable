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
	c.SendPacket(packet.NewClose(code))
	c.Connection.Close()
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
	return packet.WriteTo(c.Connection, p)
}
