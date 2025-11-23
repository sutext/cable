package server

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/listener"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Server interface {
	Serve() error
	Network() Network
	IsActive(cid string) bool
	KickConn(cid string) bool
	Shutdown(ctx context.Context) error
	SendMessage(cid string, p *packet.Message) error
	SendRequest(ctx context.Context, cid string, p *packet.Request) (*packet.Response, error)
}
type server struct {
	conns          sync.Map
	closed         atomic.Bool
	logger         logger.Logger
	address        string
	network        Network
	listener       listener.Listener
	closeHandler   ClosedHandler
	connectHander  ConnectHandler
	messageHandler MessageHandler
	requestHandler RequestHandler
}

func New(address string, opts ...Option) *server {
	options := NewOptions(opts...)
	s := &server{
		address:        address,
		network:        options.Network,
		logger:         options.Logger,
		closeHandler:   options.CloseHandler,
		connectHander:  options.ConnectHandler,
		messageHandler: options.MessageHandler,
		requestHandler: options.RequestHandler,
	}
	return s
}

func (s *server) Serve() error {
	switch s.network {
	case NetworkTCP:
		s.listener = listener.NewTCP()
	case NetworkUDP:
		s.listener = listener.NewUDP()
	default:
		return ErrNetworkNotSupport
	}
	s.listener.OnAccept(func(p *packet.Connect, c listener.Conn) packet.ConnectCode {
		return s.onConnect(newConn(c, s.logger), p)
	})
	s.listener.OnPacket(func(p packet.Packet, id *packet.Identity) {
		if s.closed.Load() {
			return
		}
		if c, ok := s.conns.Load(id.ClientID); ok {
			s.onPacket(p, c.(*conn))
		}
	})
	return s.listener.Listen(s.address)
}
func (s *server) IsActive(cid string) bool {
	if c, ok := s.conns.Load(cid); ok {
		return !c.(*conn).isClosed()
	}
	return false
}
func (s *server) KickConn(cid string) bool {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).closeCode(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return true
	}
	return false
}
func (s *server) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.listener.Close(ctx)
	for {
		activeConn := 0
		s.conns.Range(func(key, value any) bool {
			if conn, ok := value.(*conn); ok {
				if conn.isIdle() {
					conn.close()
				} else {
					activeConn++
				}
			}
			return true
		})
		if activeConn == 0 { // all connections have been closed
			return nil
		}
		// smart control graceful shutdown check internal
		// we should wait for more time if there are more active connections
		waitTime := time.Millisecond * time.Duration(activeConn)
		if waitTime > time.Second { // max wait time is 1000 ms
			waitTime = time.Millisecond * 1000
		} else if waitTime < time.Millisecond*50 { // min wait time is 50 ms
			waitTime = time.Millisecond * 50
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			continue
		}
	}
}
func (s *server) Network() Network {
	return s.network
}
func (s *server) SendMessage(cid string, p *packet.Message) error {
	if s.closed.Load() {
		return ErrServerIsClosed
	}
	if c, ok := s.conns.Load(cid); ok {
		return c.(*conn).sendMessage(p)
	}
	return ErrConnectionNotFound
}
func (s *server) SendRequest(ctx context.Context, cid string, p *packet.Request) (*packet.Response, error) {
	if s.closed.Load() {
		return nil, ErrServerIsClosed
	}
	if c, ok := s.conns.Load(cid); ok {
		return c.(*conn).sendRequest(ctx, p)
	}
	return nil, ErrConnectionNotFound
}

// Below is the code of /Users/vidar/github/cable/server/conn.go
func (s *server) onPacket(p packet.Packet, c *conn) {
	switch p.Type() {
	case packet.MESSAGE:
		s.onMessage(c, p.(*packet.Message))
	case packet.MESSACK:
		c.recvMessack(p.(*packet.Messack))
	case packet.REQUEST:
		s.onRequest(c, p.(*packet.Request))
	case packet.RESPONSE:
		c.recvResponse(p.(*packet.Response))
	case packet.PING:
		c.sendPacket(packet.NewPong())
	case packet.PONG:
		break
	case packet.CLOSE:
		c.close()
	default:
		break
	}
}

func (s *server) onConnect(c *conn, p *packet.Connect) packet.ConnectCode {
	code := s.connectHander(p)
	if code == packet.ConnectAccepted {
		if old, loaded := s.conns.Swap(p.Identity.ClientID, c); loaded {
			old.(*conn).closeCode(packet.CloseDuplicateLogin)
		}
	}
	return code
}
func (s *server) onMessage(c *conn, p *packet.Message) {
	err := s.messageHandler(p, c.ID())
	if err != nil {
		c.logger.Error("failed to handle message", "error", err)
		return
	}
	if p.Qos == packet.MessageQos1 {
		if err := c.sendPacket(packet.NewMessack(p.ID)); err != nil {
			c.logger.Error("failed to send messack", "error", err)
		}
	}
}
func (s *server) onRequest(c *conn, p *packet.Request) {
	res, err := s.requestHandler(p, c.ID())
	if err != nil {
		c.logger.Error("failed to handle request", "error", err)
		return
	}
	if err := c.sendPacket(res); err != nil {
		c.logger.Error("failed to send response", "error", err)
	}
}
func (s *server) onClose(c *conn) {
	id := c.ID()
	if id != nil {
		s.conns.Delete(id.ClientID)
		s.closeHandler(id)
	}
}
