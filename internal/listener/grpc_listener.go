package listener

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"sutext.github.io/cable/internal/listener/pb"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type grpcListener struct {
	pb.UnimplementedBytesServiceServer
	conns         safe.Map[string, *Conn]
	logger        *xlog.Logger
	listener      *net.TCPListener
	closeHandler  func(c *Conn)
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(p *packet.Connect, c *Conn) packet.ConnectCode
}

func NewGRPC(logger *xlog.Logger) Listener {
	return &grpcListener{
		logger: logger,
	}
}
func (l *grpcListener) OnClose(handler func(c *Conn)) {
	l.closeHandler = handler
}
func (l *grpcListener) OnAccept(handler func(*packet.Connect, *Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *grpcListener) OnPacket(handler func(p packet.Packet, c *Conn)) {
	l.packetHandler = handler
}
func (l *grpcListener) Close(ctx context.Context) error {
	return l.listener.Close()
}
func (l *grpcListener) Listen(address string) error {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	l.listener = listener
	gs := grpc.NewServer()
	pb.RegisterBytesServiceServer(gs, l)
	gs.Serve(listener)
	return nil
}
func (l *grpcListener) Connect(bidi grpc.BidiStreamingServer[pb.Bytes, pb.Bytes]) error {
	for {
		bytes, err := bidi.Recv()
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(bytes.Data)
		if err != nil {
			return err
		}
		timer := time.AfterFunc(time.Second*10, func() {
			l.logger.Warn("wait conn packet timeout")
		})
		timer.Stop()
		if p.Type() != packet.CONNECT {
			connID, ok := p.Get(packet.PropertyConnID)
			if !ok {
				l.logger.Warn("unknown grpc packet", xlog.Str("packet", p.String()))
				continue
			}
			c, loaded := l.conns.Get(connID)
			if !loaded {
				l.logger.Warn("unknown grpc connID", xlog.Str("connID", connID))
				continue
			}
			go l.packetHandler(p, c)
			continue
		}
		connPacket := p.(*packet.Connect)
		connId := genConnId(connPacket.Identity.ClientID)
		gc := newGRPCConn(connPacket.Identity, bidi)
		c := newConn(gc)
		gc.closeHandler = func() {
			l.conns.Delete(connId)
			if l.closeHandler != nil {
				l.closeHandler(c)
			}
		}
		code := l.acceptHandler(connPacket, c)
		if code != packet.ConnectAccepted {
			c.ClosePacket(packet.NewConnack(code))
			continue
		}
		ack := packet.NewConnack(packet.ConnectAccepted)
		ack.Set(packet.PropertyConnID, connId)
		c.SendPacket(context.Background(), ack)
		l.conns.Set(connId, c)
	}
}

type grpcConn struct {
	id           *packet.Identity
	raw          grpc.BidiStreamingServer[pb.Bytes, pb.Bytes]
	closed       atomic.Bool
	closeHandler func()
}

func (c *grpcConn) ID() *packet.Identity {
	return c.id
}
func (c *grpcConn) close() error {
	if c.closed.CompareAndSwap(false, true) {
		if c.closeHandler != nil {
			c.closeHandler()
		}
	}
	return nil
}
func (c *grpcConn) isClosed() bool {
	return c.closed.Load()
}
func (c *grpcConn) writePacket(ctx context.Context, p packet.Packet, jump bool) error {
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	err = c.raw.Send(&pb.Bytes{Data: data})
	if err != nil {
		c.close()
	}
	return err
}
func newGRPCConn(id *packet.Identity, raw grpc.BidiStreamingServer[pb.Bytes, pb.Bytes]) *grpcConn {
	return &grpcConn{
		id:  id,
		raw: raw,
	}
}
