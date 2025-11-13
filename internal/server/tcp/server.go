package tcp

import (
	"context"
	"net"
	"sync"
	"time"

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
		timer := time.AfterFunc(time.Second*10, func() {
			c.Close(packet.CloseAuthenticationTimeout)
		})
		code := s.waitConnect(c)
		timer.Stop()
		if code != 0 {
			c.Close(code)
			return code
		}
		go c.recv()
	}
}
func (s *tcpServer) GetConn(cid string) (server.Conn, bool) {
	if cn, ok := s.conns.Load(cid); ok {
		return cn.(*conn), true
	}
	return nil, false
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
func (s *tcpServer) Network() server.Network {
	return server.NetworkTCP
}
func (s *tcpServer) waitConnect(c *conn) packet.CloseCode {
	pkt, err := packet.ReadFrom(c.raw)
	if err != nil {
		switch err.(type) {
		case packet.Error:
			return packet.CloseInvalidPacket
		default:
			return packet.CloseInternalError
		}
	}
	connPacket, ok := pkt.(*packet.Connect)
	if !ok {
		return packet.CloseInternalError
	}
	code := s.connectHander(s, connPacket)
	if code != packet.ConnectionAccepted {
		return packet.CloseAuthenticationFailure
	}
	c.id = connPacket.Identity
	if old, loaded := s.conns.Swap(c.id.ClientID, c); loaded {
		old.(*conn).Close(packet.CloseDuplicateLogin)
	}
	c.sendPacket(packet.NewConnack(packet.ConnectionAccepted))
	return 0
}
func (s *tcpServer) delConn(c *conn) {
	id := c.ID()
	if id != nil {
		s.conns.Delete(id.ClientID)
	}
}
