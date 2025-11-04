package quic

import (
	"context"
	"sync"

	qc "golang.org/x/net/quic"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type quicServer struct {
	conns       sync.Map
	logger      logger.Logger
	dataHandler server.DataHandler
	connHandler server.ConnHandler
	address     string
	config      *qc.Config
}

func NewQUIC(options *server.Options) *quicServer {
	s := &quicServer{
		address: options.Address,
		config:  options.QuicConfig,
		logger:  options.Logger,
	}
	return s
}

func (s *quicServer) Serve() error {
	listener, err := qc.Listen("udp", s.address, s.config)
	if err != nil {
		return err
	}
	for {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			return err
		}
		c := newConn(conn, s)
		go c.serve()
	}
}
func (s *quicServer) HandleConn(handler server.ConnHandler) {
	s.connHandler = handler
}
func (s *quicServer) HandleData(handler server.DataHandler) {
	s.dataHandler = handler
}
func (s *quicServer) GetConn(cid string) (server.Conn, error) {
	if cn, ok := s.conns.Load(cid); ok {
		return cn.(*conn), nil
	}
	return nil, server.ErrConnctionNotFound
}
func (s *quicServer) KickConn(cid string) error {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return nil
	}
	return server.ErrConnctionNotFound
}
func (s *quicServer) Shutdown(ctx context.Context) error {
	return nil
}
func (s *quicServer) addConn(c *conn) {
	if cn, loaded := s.conns.LoadOrStore(c.GetID().ClientID, c); loaded {
		cn.(*conn).Close(packet.CloseDuplicateLogin)
	}
}
func (s *quicServer) delConn(c *conn) {
	s.conns.Delete(c.GetID().ClientID)
}
