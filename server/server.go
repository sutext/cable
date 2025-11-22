package server

import (
	"context"
	"errors"
	"sync"

	"sutext.github.io/cable/internal/listener"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Server interface {
	Serve() error
	Network() Network
	GetConn(cid string) (Conn, bool)
	KickConn(cid string) bool
	Shutdown(ctx context.Context) error
}
type server struct {
	conns          sync.Map
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
		return errors.New("unsupported network")
	}
	s.listener.OnAccept(func(p *packet.Connect, c listener.Conn) packet.ConnectCode {
		return s.onConnect(newConn(c, s.logger), p)
	})
	s.listener.OnPacket(func(p packet.Packet, id *packet.Identity) {
		if co, ok := s.conns.Load(id.ClientID); ok {
			s.onPacket(p, co.(*conn))
		}
	})
	return s.listener.Listen(s.address)
}
func (s *server) GetConn(cid string) (Conn, bool) {
	if cn, ok := s.conns.Load(cid); ok {
		return cn.(*conn), true
	}
	return nil, false
}
func (s *server) KickConn(cid string) bool {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return true
	}
	return false
}
func (s *server) Shutdown(ctx context.Context) error {
	return nil
}
func (s *server) Network() Network {
	return s.network
}
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
		c.SendPong()
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
			old.(*conn).Close(packet.CloseDuplicateLogin)
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
