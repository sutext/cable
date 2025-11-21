package udp

import (
	"context"
	"net"
	"sync"

	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type udpServer struct {
	addrs          sync.Map
	conns          sync.Map
	logger         logger.Logger
	address        string
	closeHandler   server.ClosedHandler
	connectHander  server.ConnectHandler
	messageHandler server.MessageHandler
	requestHandler server.RequestHandler
}

func NewUDP(address string, options *server.Options) *udpServer {
	s := &udpServer{
		address:        address,
		logger:         options.Logger,
		closeHandler:   options.CloseHandler,
		connectHander:  options.ConnectHandler,
		messageHandler: options.MessageHandler,
		requestHandler: options.RequestHandler,
	}
	return s
}

func (s *udpServer) Serve() error {
	addr, err := net.ResolveUDPAddr("udp4", s.address)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	for {
		buf := make([]byte, 2048)
		n, addr, err := listener.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		pkt, err := packet.Unmarshal(buf[:n])
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
			if c, loaded := s.addrs.Load(addr.String()); loaded {
				c.(*conn).handlePacket(pkt)
			}
			continue
		}
		code := s.connectHander(connPacket)
		if code != packet.ConnectAccepted {
			continue
		}
		c := newConn(connPacket.Identity, listener, addr, s.logger, s)
		if old, loaded := s.conns.Swap(connPacket.Identity.ClientID, c); loaded {
			old.(*conn).Close(packet.CloseDuplicateLogin)
		}
		s.addrs.Store(addr.String(), c)
		c.sendPacket(packet.NewConnack(packet.ConnectAccepted))
	}
}
func (s *udpServer) GetConn(cid string) (server.Conn, bool) {
	if cn, ok := s.conns.Load(cid); ok {
		return cn.(*conn), true
	}
	return nil, false
}
func (s *udpServer) KickConn(cid string) bool {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return true
	}
	return false
}
func (s *udpServer) Shutdown(ctx context.Context) error {
	return nil
}
func (s *udpServer) Network() server.Network {
	return server.NetworkUDP
}

func (s *udpServer) onClose(c *conn) {
	id := c.ID()
	if id != nil {
		s.conns.Delete(id.ClientID)
		s.closeHandler(id)
	}
}
func (s *udpServer) onConnect(c *conn, p *packet.Connect) packet.ConnectCode {
	code := s.connectHander(p)
	if code == packet.ConnectAccepted {
		if old, loaded := s.conns.Swap(p.Identity.ClientID, c); loaded {
			old.(*conn).Close(packet.CloseDuplicateLogin)
		}
	}
	return code
}
func (s *udpServer) onMessage(c *conn, p *packet.Message) {
	s.messageHandler(p, c.id)
}
func (s *udpServer) onRequest(c *conn, p *packet.Request) {
	res, err := s.requestHandler(p, c.id)
	if err != nil {
		c.logger.Error("failed to handle request", "error", err)
		return
	}
	if err := c.sendPacket(res); err != nil {
		c.logger.Error("failed to send response", "error", err)
	}
}
