package udp

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

type connHandler interface {
	onClose(c *conn)
	onConnect(c *conn, p *packet.Connect) packet.ConnectCode
	onMessage(c *conn, p *packet.Message)
	onRequest(c *conn, p *packet.Request)
}
type conn struct {
	id           *packet.Identity
	raw          *net.UDPConn
	addr         *net.UDPAddr
	closed       atomic.Bool
	logger       logger.Logger
	handler      connHandler
	sendQueue    *queue.Queue
	recvQueue    *queue.Queue
	requestTasks sync.Map
	messageTasks sync.Map
}

func newConn(id *packet.Identity, raw *net.UDPConn, addr *net.UDPAddr, logger logger.Logger, handler connHandler) *conn {
	c := &conn{
		id:        id,
		raw:       raw,
		addr:      addr,
		logger:    logger,
		handler:   handler,
		sendQueue: queue.New(1024),
		recvQueue: queue.New(1024),
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

func (c *conn) SendPing() error {
	return c.sendPacket(packet.NewPing())
}
func (c *conn) SendPong() error {
	return c.sendPacket(packet.NewPong())
}
func (c *conn) SendMessage(ctx context.Context, p *packet.Message) error {
	if p.Qos == packet.MessageQos0 {
		return c.sendPacket(p)
	}
	resp := make(chan struct{})
	defer close(resp)
	c.messageTasks.Store(p.ID, resp)
	defer c.messageTasks.Delete(p.ID)
	if err := c.sendPacket(p); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case <-resp:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
func (c *conn) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	resp := make(chan *packet.Response)
	defer close(resp)
	c.requestTasks.Store(p.ID, resp)
	defer c.requestTasks.Delete(p.ID)
	if err := c.sendPacket(p); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}
func (c *conn) close() {
	if c.closed.CompareAndSwap(false, true) {
		c.sendQueue.Close()
		c.recvQueue.Close()
		c.handler.onClose(c)
		c.handler = nil
	}
}

func (c *conn) handlePacket(p packet.Packet) {
	c.recvQueue.AddTask(func() {
		if c.closed.Load() {
			return
		}
		switch p.Type() {
		case packet.MESSAGE:
			c.handler.onMessage(c, p.(*packet.Message))
		case packet.MESSACK:
			p := p.(*packet.Messack)
			if resp, ok := c.messageTasks.Load(p.ID); ok {
				resp.(chan struct{}) <- struct{}{}
			} else {
				c.logger.Error("unexpected messack packet")
			}
		case packet.REQUEST:
			c.handler.onRequest(c, p.(*packet.Request))
		case packet.RESPONSE:
			p := p.(*packet.Response)
			if resp, ok := c.requestTasks.Load(p.ID); ok {
				resp.(chan *packet.Response) <- p
			} else {
				c.logger.Error("unexpected response packet")
			}
		case packet.PING:
			c.SendPong()
		case packet.PONG:
			break
		case packet.CLOSE:
			c.close()
		default:
			break
		}
	})
}

func (c *conn) sendPacket(p packet.Packet) error {
	return c.sendQueue.AddTask(func() {
		if c.closed.Load() {
			return
		}
		data, err := packet.Marshal(p)
		if err != nil {
			c.logger.Error("failed to marshal packet", "error", err)
			return
		}
		_, err = c.raw.WriteToUDP(data, c.addr)
		if err != nil {
			c.logger.Error("failed to send packet", "error", err)
			c.close()
		}
	})
}
