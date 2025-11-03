package quic

import (
	"context"

	qc "golang.org/x/net/quic"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type quicServer struct {
	conns   *safe.Map[string, *conn]
	logger  logger.Logger
	onData  server.OnData
	onAuth  server.OnAuth
	address string
	config  *qc.Config
}

func NewQUIC(options *server.Options) *quicServer {
	s := &quicServer{
		conns:   safe.NewMap(map[string]*conn{}),
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
func (s *quicServer) OnAuth(handler server.OnAuth) {
	s.onAuth = handler
}
func (s *quicServer) OnData(handler server.OnData) {
	s.onData = handler
}
func (s *quicServer) GetConn(cid string) (server.Conn, error) {
	if conn, ok := s.conns.Get(cid); ok {
		return conn, nil
	}
	return nil, server.ErrConnctionNotFound
}
func (s *quicServer) KickConn(cid string) error {
	if conn, ok := s.conns.Get(cid); ok {
		conn.Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return nil
	}
	return server.ErrConnctionNotFound
}
func (s *quicServer) Shutdown(ctx context.Context) error {
	return nil
}
func (s *quicServer) addConn(c *conn) {
	s.conns.Write(func(m map[string]*conn) {
		cid := c.GetID().ClientID
		if old, ok := m[cid]; ok {
			old.Close(packet.CloseDuplicateLogin)
		}
		m[cid] = c
	})
}
func (s *quicServer) delConn(c *conn) {
	s.conns.Delete(c.GetID().ClientID)
}
