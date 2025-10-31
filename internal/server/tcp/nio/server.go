package nio

import (
	"context"
	"net"

	"github.com/cloudwego/netpoll"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type nioServer struct {
	conns     *safe.Map[string, *conn]
	onData    server.OnData
	onAuth    server.OnAuth
	address   string
	eventLoop netpoll.EventLoop
}

func NewNIO(options *server.Options) *nioServer {
	s := &nioServer{
		address: options.Address,
		conns:   safe.NewMap[string, *conn](),
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
func (s *nioServer) OnAuth(handler server.OnAuth) {
	s.onAuth = handler
}
func (s *nioServer) OnData(handler server.OnData) {
	s.onData = handler
}

func (s *nioServer) GetConn(cid string) (server.Conn, error) {
	if con, ok := s.conns.Get(cid); ok {
		return con, nil
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
	if conn, ok := s.conns.Get(cid); ok {
		conn.Close(packet.CloseKickedOut)
		s.conns.Delete(cid)
		return nil
	}
	return server.ErrConnctionNotFound
}

func (s *nioServer) handlePacket(id *packet.Identity, p packet.Packet) {
	conn, ok := s.conns.Get(id.ClientID)
	if !ok {
		return
	}
	switch p.Type() {
	case packet.DATA:
		p := p.(*packet.DataPacket)
		if s.onData == nil {
			return
		}
		res, err := s.onData(id.UserID, p)
		if err != nil {
			return
		}
		if res != nil {
			conn.sendPacket(res)
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
	if s.onAuth == nil {
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
	if err := s.onAuth(connPacket.Identity); err != nil {
		closePacket := &packet.ClosePacket{Code: packet.CloseAuthenticationFailure}
		packet.WriteTo(c, closePacket)
		c.Close()
		return ctx
	}
	s.conns.Set(connPacket.Identity.ClientID, &conn{
		Connection: c,
		id:         connPacket.Identity,
	})
	packet.WriteTo(c, packet.NewConnack(packet.ConnectionAccepted))
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
