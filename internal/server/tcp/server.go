package tcp

import (
	"context"
	"net"

	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type bioServer struct {
	conns   *safe.Map[string, *conn]
	logger  logger.Logger
	onData  server.OnData
	onAuth  server.OnAuth
	address string
}

func NewTCP(options *server.Options) *bioServer {
	s := &bioServer{
		conns:   safe.NewMap(map[string]*conn{}),
		address: options.Address,
	}
	return s
}

func (s *bioServer) Serve() error {
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
func (s *bioServer) OnAuth(handler server.OnAuth) {
	s.onAuth = handler
}
func (s *bioServer) OnData(handler server.OnData) {
	s.onData = handler
}
func (s *bioServer) GetConn(cid string) (server.Conn, error) {
	if conn, ok := s.conns.Get(cid); ok {
		return conn, nil
	}
	return nil, server.ErrConnctionNotFound
}
func (s *bioServer) KickConn(cid string) error {
	if conn, ok := s.conns.Get(cid); ok {
		conn.Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return nil
	}
	return server.ErrConnctionNotFound
}
func (s *bioServer) Shutdown(ctx context.Context) error {
	return nil
}
func (s *bioServer) addConn(c *conn) {
	s.conns.Write(func(m map[string]*conn) {
		cid := c.GetID().ClientID
		if old, ok := m[cid]; ok {
			old.Close(packet.CloseDuplicateLogin)
		}
		m[cid] = c
	})
}
func (s *bioServer) delConn(c *conn) {
	s.conns.Delete(c.GetID().ClientID)
}
