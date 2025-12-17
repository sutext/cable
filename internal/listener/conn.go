package listener

import (
	"context"
	"sync"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Conn struct {
	raw          conn
	logger       *xlog.Logger
	requestTasks sync.Map
	messageTasks sync.Map
}

func newConn(raw conn) *Conn {
	c := &Conn{
		raw:    raw,
		logger: xlog.With("GROUP", "SERVER"),
	}
	return c
}

func (c *Conn) ID() *packet.Identity {
	return c.raw.ID()
}

func (c *Conn) IsIdle() bool {
	return true
}

func (c *Conn) IsClosed() bool {
	return c.raw.isClosed()
}
func (c *Conn) SendQueueLength() int32 {
	return c.raw.sendQueueLength()
}
func (c *Conn) SendMessage(ctx context.Context, p *packet.Message) error {
	if p.Qos == packet.MessageQos0 {
		return c.SendPacket(ctx, p)
	}
	return c.retryInflightMessage(ctx, p, 0)
}
func (c *Conn) retryInflightMessage(ctx context.Context, p *packet.Message, attempts int) error {
	if attempts > 5 {
		return xerr.MessageTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	_, err := c.sendInflightMessage(ctx, p)
	if err != nil {
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			return c.retryInflightMessage(context.Background(), p, attempts+1)
		}
		return err
	}
	return nil
}
func (c *Conn) sendInflightMessage(ctx context.Context, p *packet.Message) (*packet.Messack, error) {
	if c.IsClosed() {
		return nil, xerr.ConnectionIsClosed
	}
	akcCh := make(chan *packet.Messack)
	defer close(akcCh)
	c.messageTasks.Store(p.ID, akcCh)
	defer c.messageTasks.Delete(p.ID)
	if err := c.SendPacket(ctx, p); err != nil {
		return nil, err
	}
	select {
	case ack := <-akcCh:
		return ack, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *Conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if c.IsClosed() {
		return nil, xerr.ConnectionIsClosed
	}
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.ID, resp)
	defer c.requestTasks.Delete(p.ID)
	if err := c.SendPacket(ctx, p); err != nil {
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
	if ackCh, ok := c.messageTasks.Load(p.ID); ok {
		ackCh.(chan *packet.Messack) <- p
	}
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
	return c.jumpPacket(context.Background(), p)
}
func (c *Conn) DuplicateClose() error {
	p := packet.NewClose(packet.CloseDuplicateLogin)
	return c.jumpPacket(context.Background(), p)
}
func (c *Conn) SendPong() error {
	return c.jumpPacket(context.Background(), packet.NewPong())
}
func (c *Conn) SendPacket(ctx context.Context, p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	return c.raw.writePacket(ctx, p, false)
}
func (c *Conn) jumpPacket(ctx context.Context, p packet.Packet) error {
	if c.IsClosed() {
		return xerr.ConnectionIsClosed
	}
	return c.raw.writePacket(ctx, p, true)
}
