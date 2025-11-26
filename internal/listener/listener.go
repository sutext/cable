package listener

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/inflight"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
)

type Listener interface {
	Listen(addr string) error
	Close(ctx context.Context) error
	OnPacket(handler func(p packet.Packet, c *Conn))
	OnAccept(handler func(p *packet.Connect, c *Conn) packet.ConnectCode)
}
type Conn struct {
	raw          conn
	closed       atomic.Bool
	recvQueue    *queue.Queue
	inflights    *inflight.Inflight
	closeHandler func(c *Conn)
	requestTasks sync.Map
}

func newConn(raw conn) *Conn {
	c := &Conn{
		raw:       raw,
		recvQueue: queue.New(1024),
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
	return c.recvQueue.IsIdle()
}
func (c *Conn) OnClose(handler func(c *Conn)) {
	c.closeHandler = handler
}
func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}
func (c *Conn) CloseCode(code packet.CloseCode) {
	c.SendPacket(packet.NewClose(code))
	c.Close()
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
func (c *Conn) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.recvQueue.Close()
		c.raw.close()
		if c.closeHandler != nil {
			c.closeHandler(c)
		}
	}
}

func (c *Conn) SendPacket(p packet.Packet) error {
	if c.closed.Load() {
		return fmt.Errorf("connection is closed")
	}
	err := c.raw.writePacket(p)
	if err != nil {
		switch err.(type) {
		case packet.Error, coder.Error:
			break
		default:
			c.Close()
		}
	}
	return err
}
