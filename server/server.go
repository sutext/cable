package server

import (
	"context"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/listener"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Server interface {
	// Serve starts the server and listens for incoming connections.
	Serve() error
	/// IsActive returns true if the client is active.
	IsActive(cid string) bool
	// KickConn kicks a client out of the server.
	KickConn(cid string) bool
	// Shutdown shuts down the server and closes all connections.
	Shutdown(ctx context.Context) error
	// Brodcast sends a message to all clients.
	Brodcast(ctx context.Context, p *packet.Message) (total, success int32, err error)
	// Transport returns the transport of the server.
	Transport() Transport
	// SendMessage sends a message to a client.
	SendMessage(ctx context.Context, cid string, p *packet.Message) error
	// SendRequest sends a request to a client and returns the response.
	SendRequest(ctx context.Context, cid string, p *packet.Request) (*packet.Response, error)
}
type server struct {
	conns          safe.Map[string, listener.Conn]
	logger         *xlog.Logger
	closed         atomic.Bool
	address        string
	listener       listener.Listener
	transport      Transport
	queueCapacity  int32
	closeHandler   ClosedHandler
	connectHandler ConnectHandler
	messageHandler MessageHandler
	requestHandler RequestHandler
}

func New(address string, opts ...Option) Server {
	options := NewOptions(opts...)
	s := &server{
		logger:         options.logger,
		address:        address,
		transport:      options.transport,
		queueCapacity:  options.queueCapacity,
		closeHandler:   options.closeHandler,
		connectHandler: options.connectHandler,
		messageHandler: options.messageHandler,
		requestHandler: options.requestHandler,
	}
	return s
}

func (s *server) Serve() error {
	switch s.transport {
	case TransportTCP:
		s.listener = listener.NewTCP(s.logger, s.queueCapacity)
	case TransportUDP:
		s.listener = listener.NewUDP(s.logger, s.queueCapacity)
	case TransportGRPC:
		s.listener = listener.NewGRPC(s.logger, s.queueCapacity)
	default:
		return xerr.TransportNotSupported
	}
	s.listener.OnClose(s.onClose)
	s.listener.OnAccept(s.onConnect)
	s.listener.OnPacket(s.onPacket)
	s.logger.Info("server listening", xlog.Str("address", s.address), xlog.Str("transport", string(s.transport)))
	return s.listener.Listen(s.address)
}
func (s *server) IsActive(cid string) bool {
	if c, ok := s.conns.Get(cid); ok {
		return !c.IsClosed()
	}
	return false
}
func (s *server) KickConn(cid string) bool {
	if c, ok := s.conns.Get(cid); ok {
		c.CloseClode(packet.CloseKickedOut)
		return true
	}
	return false
}
func (s *server) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return xerr.ServerAlreadyClosed
	}
	err := s.listener.Close(ctx)
	for {
		activeConn := 0
		s.conns.Range(func(key string, conn listener.Conn) bool {
			if conn.IsIdle() {
				conn.Close()
			} else {
				activeConn++
			}
			return true
		})
		if activeConn == 0 { // all connections have been closed
			return err
		}
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
func (s *server) Brodcast(ctx context.Context, p *packet.Message) (int32, int32, error) {
	if s.closed.Load() {
		return 0, 0, xerr.ServerIsClosed
	}
	var total, success int32
	s.conns.Range(func(cid string, conn listener.Conn) bool {
		total++
		if err := conn.SendMessage(ctx, p); err == nil {
			success++
		}
		return true
	})
	return total, success, nil
}
func (s *server) Transport() Transport {
	return s.transport
}
func (s *server) SendMessage(ctx context.Context, cid string, p *packet.Message) error {
	if s.closed.Load() {
		return xerr.ServerIsClosed
	}
	if c, ok := s.conns.Get(cid); ok {
		return c.SendMessage(ctx, p)
	}
	return xerr.ConnectionNotFound
}
func (s *server) SendRequest(ctx context.Context, cid string, p *packet.Request) (*packet.Response, error) {
	if s.closed.Load() {
		return nil, xerr.ServerIsClosed
	}
	if c, ok := s.conns.Get(cid); ok {
		return c.SendRequest(ctx, p)
	}
	return nil, xerr.ConnectionNotFound
}

// Below is the code of /Users/vidar/github/cable/server/conn.go
func (s *server) onPacket(p packet.Packet, c listener.Conn) {
	if s.closed.Load() {
		return
	}
	switch p.Type() {
	case packet.MESSAGE:
		s.onMessage(c, p.(*packet.Message))
	case packet.MESSACK:
		c.RecvMessack(p.(*packet.Messack))
	case packet.REQUEST:
		s.onRequest(c, p.(*packet.Request))
	case packet.RESPONSE:
		c.RecvResponse(p.(*packet.Response))
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		c.RecvPong()
	case packet.CLOSE:
		c.Close()
	default:
		break
	}
}

// Below is the code of
func (s *server) onConnect(p *packet.Connect, c listener.Conn) packet.ConnectCode {
	code := s.connectHandler(p)
	if code == packet.ConnectAccepted {
		if old, loaded := s.conns.Swap(p.Identity.ClientID, c); loaded {
			old.CloseClode(packet.CloseDuplicateLogin)
		}
	}
	return code
}
func (s *server) onMessage(c listener.Conn, p *packet.Message) {
	err := s.messageHandler(p, c.ID())
	if err != nil {
		s.logger.Error("failed to handle message", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		return
	}
	if p.Qos == packet.MessageQos1 {
		if err := c.SendPacket(context.Background(), packet.NewMessack(p.ID)); err != nil {
			s.logger.Error("failed to send messack", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		}
	}
}
func (s *server) onRequest(c listener.Conn, p *packet.Request) {
	res, err := s.requestHandler(p, c.ID())
	if err != nil {
		s.logger.Error("failed to handle request", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		return
	}
	if err := c.SendPacket(context.Background(), res); err != nil {
		s.logger.Error("failed to send response", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
	}
}
func (s *server) onClose(c listener.Conn) {
	id := c.ID()
	if id == nil {
		return
	}
	if s.conns.Delete(id.ClientID) {
		s.closeHandler(id)
	}
}
