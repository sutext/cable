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
	conns          sync.Map
	logger         logger.Logger
	address        string
	eventLoop      netpoll.EventLoop
	connectHander  server.ConnectHandler
	messageHandler server.MessageHandler
	requestHandler server.RequestHandler
}

func NewNIO(address string, options *server.Options) *nioServer {
	s := &nioServer{
		address:        address,
		logger:         options.Logger,
		connectHander:  options.ConnectHandler,
		messageHandler: options.MessageHandler,
		requestHandler: options.RequestHandler,
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
func (s *nioServer) Network() server.Network {
	return server.NetworkTCP
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
	case packet.MESSAGE:
		s.messageHandler(s, p.(*packet.Message), id)
	case packet.REQUEST:
		p := p.(*packet.Request)
		res, err := s.requestHandler(s, p, id)
		if err != nil {
			return
		}
		if err := conn.sendPacket(res); err != nil {
			s.logger.Error("send response error: %v", err)
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		conn.handleResponse(p)
	case packet.PING:
		conn.SendPong()
	default:
		break
	}
}

func (s *nioServer) onRequest(ctx context.Context, conn netpoll.Connection) error {
	id := ctx.Value(identityKey).(*packet.Identity)
	pkt, err := packet.ReadFrom(netpoll.NewIOReader(conn.Reader()))
	if err != nil {
		switch err.(type) {
		case packet.Error:
			s.close(conn, packet.CloseInvalidPacket)
		default:
			s.close(conn, packet.CloseInternalError)
		}
		return err
	}
	s.handlePacket(id, pkt)
	return nil
}
func (s *nioServer) onPrepare(conn netpoll.Connection) context.Context {
	return context.Background()
}
func (s *nioServer) onConnect(ctx context.Context, c netpoll.Connection) context.Context {
	pkt, err := packet.ReadFrom(netpoll.NewIOReader(c.Reader()))
	if err != nil {
		switch err.(type) {
		case packet.Error:
			s.close(c, packet.CloseInvalidPacket)
		default:
			s.close(c, packet.CloseInternalError)
		}
		return ctx
	}
	connPacket, ok := pkt.(*packet.Connect)
	if !ok {
		s.close(c, packet.CloseInternalError)
		return ctx
	}
	code := s.connectHander(s, connPacket)
	if code == packet.ConnectionAccepted {
		cn := &conn{
			Connection: c,
			id:         connPacket.Identity,
		}
		if old, loaded := s.conns.Swap(connPacket.Identity.ClientID, cn); loaded {
			old.(*conn).Close(packet.CloseDuplicateLogin)
		}
	}
	s.send(c, packet.NewConnack(code))
	if code != packet.ConnectionAccepted {
		s.close(c, packet.CloseAuthenticationFailure)
		return ctx
	}
	return context.WithValue(ctx, identityKey, connPacket.Identity)
}
func (s *nioServer) close(conn netpoll.Connection, code packet.CloseCode) {
	s.send(conn, packet.NewClose(code))
	conn.Close()
}
func (s *nioServer) send(conn netpoll.Connection, p packet.Packet) {
	w := conn.Writer()
	packet.WriteTo(netpoll.NewIOWriter(w), p)
	w.Flush()
}
func (s *nioServer) onDisconnect(ctx context.Context, conn netpoll.Connection) {
	id, ok := ctx.Value(identityKey).(*packet.Identity)
	if ok {
		s.conns.Delete(id.ClientID)
	}
}

type contextKey struct{}

var identityKey = contextKey{}
