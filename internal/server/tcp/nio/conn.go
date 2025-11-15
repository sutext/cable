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

func newConn(c netpoll.Connection) *conn {
	return &conn{
		Connection: c,
	}
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
func (c *conn) sendPacket(p packet.Packet) error {
	w := c.Connection.Writer()
	err := packet.WriteTo(netpoll.NewIOWriter(w), p)
	if err != nil {
		return err
	}
	return w.Flush()
}
func (c *conn) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.tasks.Store(p.Seq, resp)
	if err := c.sendPacket(p); err != nil {
		return nil, err
	}
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
	if resp, ok := c.tasks.LoadAndDelete(p.Seq); ok {
		(resp.(chan *packet.Response)) <- p
	}
}
