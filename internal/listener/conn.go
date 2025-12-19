package listener

import (
	"context"
	"encoding/hex"
	"hash/crc32"
	"sync"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type conn interface {
	ID() *packet.Identity
	close() error
	isClosed() bool
	writePacket(ctx context.Context, p packet.Packet, jump bool) error
}
type Listener interface {
	Close(ctx context.Context) error
	Listen(addr string) error
	OnClose(handler func(c *Conn))
	OnPacket(handler func(p packet.Packet, c *Conn))
	OnAccept(handler func(p *packet.Connect, c *Conn) packet.ConnectCode)
}

type Conn struct {
	raw          conn
	logger       *xlog.Logger
	requestLock  sync.Mutex
	messageLock  sync.Mutex
	requestTasks map[int64]chan *packet.Response
	messageTasks map[int64]chan *packet.Messack
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
	c.messageLock.Lock()
	ackCh := make(chan *packet.Messack)
	c.messageTasks[p.ID] = ackCh
	c.messageLock.Unlock()
	defer func() {
		c.messageLock.Lock()
		delete(c.messageTasks, p.ID)
		close(ackCh)
		c.messageLock.Unlock()
	}()
	if err := c.SendPacket(ctx, p); err != nil {
		return nil, err
	}
	select {
	case ack := <-ackCh:
		return ack, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *Conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if c.IsClosed() {
		return nil, xerr.ConnectionIsClosed
	}
	c.requestLock.Lock()
	resp := make(chan *packet.Response)
	c.requestTasks[p.ID] = resp
	c.requestLock.Unlock()
	defer func() {
		c.requestLock.Lock()
		delete(c.requestTasks, p.ID)
		close(resp)
		c.requestLock.Unlock()
	}()
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
	c.messageLock.Lock()
	ch, ok := c.messageTasks[p.ID]
	if ok {
		ch <- p
	}
	c.messageLock.Unlock()
	if !ok {
		c.logger.Error("response task not found", xlog.I64("id", p.ID))
	}
}
func (c *Conn) RecvResponse(p *packet.Response) {
	c.requestLock.Lock()
	ch, ok := c.requestTasks[p.ID]
	if ok {
		ch <- p
	}
	c.requestLock.Unlock()
	if !ok {
		c.logger.Error("response task not found", xlog.I64("id", p.ID))
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
func genConnId(s string) string {
	h := crc32.NewIEEE()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
