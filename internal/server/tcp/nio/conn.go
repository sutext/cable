package nio

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cloudwego/netpoll"
	"sutext.github.io/cable/packet"
)

type conn struct {
	netpoll.Connection
	id    *packet.Identity
	tasks sync.Map
}

func (c *conn) ID() *packet.Identity {
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
	return c.sendPacket(packet.NewMessage(data))
}

func (c *conn) sendPacket(p packet.Packet) error {
	return packet.WriteTo(c.Connection, p)
}
func (c *conn) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *conn) Request(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	c.sendPacket(p)
	resp := make(chan *packet.Response)
	c.tasks.Store(p.Serial, resp)
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, errors.New("timeout")
	}
}
func (c *conn) handleResponse(p *packet.Response) {
	if resp, ok := c.tasks.LoadAndDelete(p.Serial); ok {
		(resp.(chan *packet.Response)) <- p
	}
}
