package tcp

import (
	"context"
	"net"
	"sync"

	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type tcpServer struct {
	conns          sync.Map
	logger         logger.Logger
	address        string
	closeHandler   server.ClosedHandler
	connectHander  server.ConnectHandler
	messageHandler server.MessageHandler
	requestHandler server.RequestHandler
}

func NewTCP(address string, options *server.Options) *tcpServer {
	s := &tcpServer{
		address:        address,
		logger:         options.Logger,
		closeHandler:   options.CloseHandler,
		connectHander:  options.ConnectHandler,
		messageHandler: options.MessageHandler,
		requestHandler: options.RequestHandler,
	}
	return s
}

func (s *tcpServer) Serve() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		c := newConn(conn, s.logger, s)
		go c.recv()
	}
}
func (s *tcpServer) GetConn(cid string) (server.Conn, bool) {
	if cn, ok := s.conns.Load(cid); ok {
		return cn.(*conn), true
	}
	return nil, false
}
func (s *tcpServer) KickConn(cid string) bool {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return true
	}
	return false
}
func (s *tcpServer) Shutdown(ctx context.Context) error {
	return nil
}
func (s *tcpServer) Network() server.Network {
	return server.NetworkTCP
}

func (s *tcpServer) onClose(c *conn) {
	id := c.ID()
	if id != nil {
		s.conns.Delete(id.ClientID)
		s.closeHandler(s, id)
	}
}
func (s *tcpServer) onConnect(c *conn, p *packet.Connect) packet.ConnackCode {
	code := s.connectHander(s, p)
	if code == packet.ConnectionAccepted {
		if old, loaded := s.conns.Swap(p.Identity.ClientID, c); loaded {
			old.(*conn).Close(packet.CloseDuplicateLogin)
		}
	}
	return code
}
func (s *tcpServer) onMessage(c *conn, p *packet.Message) {
	s.messageHandler(s, p, c.id)
}
func (s *tcpServer) onRequest(c *conn, p *packet.Request) {
	res, err := s.requestHandler(s, p, c.id)
	if err != nil {
		c.logger.Error("failed to handle request", "error", err)
		return
	}
	if err := c.sendPacket(res); err != nil {
		c.logger.Error("failed to send response", "error", err)
	}
}
