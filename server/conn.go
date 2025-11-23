package server

import (
	"context"
	"sync"
	"sync/atomic"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/inflight"
	"sutext.github.io/cable/internal/listener"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
)

type conn struct {
	raw          listener.Conn
	closed       atomic.Bool
	logger       logger.Logger
	sendQueue    *queue.Queue
	recvQueue    *queue.Queue
	inflights    *inflight.Inflight
	requestTasks sync.Map
}

func newConn(raw listener.Conn, logger logger.Logger) *conn {
	c := &conn{
		raw:       raw,
		logger:    logger,
		sendQueue: queue.New(1024),
		recvQueue: queue.New(1024),
	}
	c.inflights = inflight.New(func(m *packet.Message) {
		c.sendPacket(m)
	})
	return c
}
func (c *conn) ID() *packet.Identity {
	return c.raw.ID()
}
func (c *conn) isIdle() bool {
	return c.sendQueue.IsIdle() && c.recvQueue.IsIdle()
}
func (c *conn) isClosed() bool {
	return c.closed.Load()
}
func (c *conn) closeCode(code packet.CloseCode) {
	c.sendPacket(packet.NewClose(code))
	c.close()
}

func (c *conn) sendMessage(p *packet.Message) error {
	if p.Qos == packet.MessageQos1 {
		c.inflights.Add(p)
	}
	return c.sendPacket(p)
}
func (c *conn) sendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.ID, resp)
	defer c.requestTasks.Delete(p.ID)
	if err := c.sendPacket(p); err != nil {
		return nil, err
	}
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *conn) recvMessack(p *packet.Messack) {
	c.inflights.Remove(p.ID)
}
func (c *conn) recvResponse(p *packet.Response) {
	if resp, ok := c.requestTasks.Load(p.ID); ok {
		resp.(chan *packet.Response) <- p
	} else {
		c.logger.Error("unexpected response packet")
	}
}
func (c *conn) close() {
	if c.closed.CompareAndSwap(false, true) {
		c.sendQueue.Close()
		c.recvQueue.Close()
		c.raw.Close()
	}
}

func (c *conn) sendPacket(p packet.Packet) error {
	return c.sendQueue.AddTask(func() {
		if c.closed.Load() {
			return
		}
		if err := c.raw.WritePacket(p); err != nil {
			c.logger.Error("failed to send packet", "error", err)
			switch err.(type) {
			case packet.Error, coder.Error:
				break
			default:
				c.close()
			}
		}
	})
}
