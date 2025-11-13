package tcp

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
)

type conn struct {
	id           *packet.Identity
	raw          net.Conn
	closed       atomic.Bool
	server       *tcpServer
	logger       logger.Logger
	sendQueue    *queue.Queue
	recvQueue    *queue.Queue
	requestTasks sync.Map
}

func newConn(raw net.Conn, server *tcpServer) *conn {
	c := &conn{
		raw:       raw,
		server:    server,
		logger:    server.logger,
		sendQueue: queue.New(512),
		recvQueue: queue.New(512, 32),
	}
	return c
}
func (c *conn) ID() *packet.Identity {
	return c.id
}
func (c *conn) IsActive() bool {
	return !c.closed.Load()
}
func (c *conn) Close(code packet.CloseCode) {
	c.sendPacket(packet.NewClose(code))
	c.close()
}

func (c *conn) close() {
	if c.closed.CompareAndSwap(false, true) {
		c.sendQueue.Stop()
		c.recvQueue.Stop()
		c.server.delConn(c)
		c.raw.Close()
		c.raw = nil
		c.server = nil
	}
}
func (c *conn) recv() {
	for {
		p, err := packet.ReadFrom(c.raw)
		if err != nil {
			switch err.(type) {
			case packet.Error:
				c.Close(packet.CloseInvalidPacket)
			default:
				c.Close(packet.CloseInternalError)
			}
			return
		}
		c.recvQueue.Push(func() {
			c.handlePacket(p)
		})
	}
}
func (c *conn) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *conn) SendMessage(p *packet.Message) error {
	return c.sendPacket(p)
}
func (c *conn) Request(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.Seq, resp)
	defer c.requestTasks.Delete(p.Seq)
	if err := c.sendPacket(p); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *conn) sendPacket(p packet.Packet) error {
	return c.sendQueue.Push(func() {
		if err := packet.WriteTo(c.raw, p); err != nil {
			c.logger.Debug("failed to send packet", "error", err)
			if _, ok := err.(net.Error); ok {
				c.close()
				return
			}
		}
	})
}

func (c *conn) handlePacket(p packet.Packet) {
	switch p.Type() {
	case packet.MESSAGE:
		c.server.messageHandler(c.server, p.(*packet.Message), c.id)
	case packet.REQUEST:
		c.handleRequest(p.(*packet.Request))
	case packet.RESPONSE:
		c.handleResponse(p.(*packet.Response))
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		break
	case packet.CLOSE:
		c.close()
		c.logger.Debug("close packet received")
	default:
		break
	}
}

func (c *conn) handleRequest(p *packet.Request) {
	res, err := c.server.requestHandler(c.server, p, c.id)
	if err != nil {
		return
	}
	if err := c.sendPacket(res); err != nil {
		c.logger.Debug("failed to send response", "error", err)
	}
}
func (c *conn) handleResponse(p *packet.Response) {
	if resp, ok := c.requestTasks.Load(p.Seq); ok {
		resp.(chan *packet.Response) <- p
	} else {
		c.logger.Debug("unexpected response packet")
	}
}
