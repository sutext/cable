package nio

import (
	"context"
	"net"
	"sync"

	"github.com/cloudwego/netpoll"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type nioServer struct {
	conns       sync.Map
	logger      logger.Logger
	dataHandler server.DataHandler
	connHandler server.ConnHandler
	address     string
	eventLoop   netpoll.EventLoop
}

func NewNIO(options *server.Options) *nioServer {
	s := &nioServer{
		address: options.Address,
		logger:  options.Logger,
	}
	return s
}
func (s *nioServer) Serve() error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	eventLoop, err := netpoll.NewEventLoop(s.onRequest,
		netpoll.WithOnPrepare(s.onPrepare),
		netpoll.WithOnConnect(s.onConnect),
		netpoll.WithOnDisconnect(s.onDisconnect),
	)
	if err != nil {
		return err
	}
	s.eventLoop = eventLoop
	return eventLoop.Serve(ln)
}
func (s *nioServer) HandleConn(handler server.ConnHandler) {
	s.connHandler = handler
}
func (s *nioServer) HandleData(handler server.DataHandler) {
	s.dataHandler = handler
}

func (s *nioServer) GetConn(cid string) (server.Conn, error) {
	if con, ok := s.conns.Load(cid); ok {
		return con.(*conn), nil
	}
	return nil, server.ErrConnctionNotFound
}
func (s *nioServer) Shutdown(ctx context.Context) error {
	if s.eventLoop != nil {
		return s.eventLoop.Shutdown(ctx)
	}
	return nil
}
func (s *nioServer) KickConn(cid string) error {
	if cn, ok := s.conns.Load(cid); ok {
		cn.(*conn).Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return nil
	}
	return server.ErrConnctionNotFound
}

func (s *nioServer) handlePacket(id *packet.Identity, p packet.Packet) {
	cn, ok := s.conns.Load(id.ClientID)
	if !ok {
		return
	}
	conn := cn.(*conn)
	switch p.Type() {
	case packet.DATA:
		p := p.(*packet.DataPacket)
		if s.dataHandler == nil {
			return
		}
		res, err := s.dataHandler(id, p)
		if err != nil {
			return
		}
		if res != nil {
			conn.SendPacket(res)
		}
	case packet.PING:
		conn.SendPong()
	default:
		break
	}
}

func (s *nioServer) onRequest(ctx context.Context, conn netpoll.Connection) error {
	id := ctx.Value(identityKey).(*packet.Identity)
	pkt, err := packet.ReadFrom(conn)
	if err != nil {
		conn.Close()
		return err
	}
	s.handlePacket(id, pkt)
	return nil
}
func (s *nioServer) onPrepare(conn netpoll.Connection) context.Context {
	return context.Background()
}
func (s *nioServer) onConnect(ctx context.Context, c netpoll.Connection) context.Context {
	if s.connHandler == nil {
		c.Close()
		return ctx
	}
	pkt, err := packet.ReadFrom(c)
	if err != nil {
		c.Close()
		return ctx
	}
	connPacket, ok := pkt.(*packet.ConnectPacket)
	if !ok {
		c.Close()
		return ctx
	}
	code := s.connHandler(connPacket)

	if code == packet.AlreadyConnected {
		cn := &conn{
			Connection: c,
			id:         connPacket.Identity,
		}
		if old, loaded := s.conns.LoadOrStore(connPacket.Identity.ClientID, cn); loaded {
			old.(*conn).Close(packet.CloseDuplicateLogin)
		}
	}
	packet.WriteTo(c, packet.NewConnack(code))
	if code != packet.AlreadyConnected {
		closePacket := &packet.ClosePacket{Code: packet.CloseAuthenticationFailure}
		packet.WriteTo(c, closePacket)
		c.Close()
		return ctx
	}
	return context.WithValue(ctx, identityKey, connPacket.Identity)
}
func (s *nioServer) onDisconnect(ctx context.Context, conn netpoll.Connection) {
	id, ok := ctx.Value(identityKey).(*packet.Identity)
	if ok {
		s.conns.Delete(id.ClientID)
	}
}

type contextKey struct{}

var identityKey = contextKey{}
