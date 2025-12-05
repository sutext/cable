package listener

import (
	"context"
	"sync"

	"sutext.github.io/cable/internal/inflight"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Conn struct {
	raw          conn
	logger       *xlog.Logger
	inflights    *inflight.Inflight
	requestTasks sync.Map
}

func newConn(raw conn) *Conn {
	c := &Conn{
		raw:    raw,
		logger: xlog.With("GROUP", "SERVER"),
	}
	c.inflights = inflight.New(func(m *packet.Message) {
		c.SendPacket(m)
	})
	return c
}

func (c *Conn) ID() *packet.Identity {
	return c.raw.ID()
}

func (c *Conn) IsIdle() bool {
	return true
}

//	func (c *Conn) OnClose(handler func(c *Conn)) {
//		c.closeHandler = handler
//	}
func (c *Conn) IsClosed() bool {
	return c.raw.isClosed()
}

func (c *Conn) SendMessage(p *packet.Message) error {
	if p.Qos == packet.MessageQos1 {
		c.inflights.Add(p)
	}
	return c.SendPacket(p)
}
func (c *Conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.ID, resp)
	defer c.requestTasks.Delete(p.ID)
	if err := c.SendPacket(p); err != nil {
		return nil, err
	}
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *Conn) RecvMessack(p *packet.Messack) {
	c.inflights.Remove(p.ID)
}
func (c *Conn) RecvResponse(p *packet.Response) {
	if resp, ok := c.requestTasks.Load(p.ID); ok {
		resp.(chan *packet.Response) <- p
	}
}
func (c *Conn) Close() error {
	return c.raw.close()
}
func (c *Conn) ClosePacket(p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	return c.raw.writePacket(p, true)
}
func (c *Conn) DuplicateClose() error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	return c.raw.writePacket(packet.NewClose(packet.CloseDuplicateLogin), true)
}
func (c *Conn) SendPong() error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	return c.raw.writePacket(packet.NewPong(), true)
}
func (c *Conn) SendPacket(p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	return c.raw.writePacket(p, false)
}
