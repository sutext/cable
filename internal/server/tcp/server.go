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
	connectHander  server.ConnectHandler
	messageHandler server.MessageHandler
	requestHandler server.RequestHandler
}

func NewTCP(address string, options *server.Options) *tcpServer {
	s := &tcpServer{
		address:        address,
		logger:         options.Logger,
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
		c := newConn(conn, s)
		go c.serve()
	}
}
func (s *tcpServer) GetConn(cid string) (server.Conn, error) {
	if cn, ok := s.conns.Load(cid); ok {
		return cn.(*conn), nil
	}
	return nil, server.ErrConnctionNotFound
}
func (s *tcpServer) KickConn(cid string) error {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return nil
	}
	return server.ErrConnctionNotFound
}
func (s *tcpServer) Shutdown(ctx context.Context) error {
	return nil
}
func (s *tcpServer) addConn(c *conn) {
	if old, loaded := s.conns.Swap(c.ID().ClientID, c); loaded {
		old.(*conn).Close(packet.CloseDuplicateLogin)
		return
	}
}
func (s *tcpServer) delConn(c *conn) {
	s.conns.Delete(c.ID().ClientID)
}
