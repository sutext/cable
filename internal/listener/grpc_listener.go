package listener

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"sutext.github.io/cable/internal/listener/pb"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type grpcListener struct {
	pb.UnimplementedBytesServiceServer
	conns         safe.RMap[string, Conn]
	logger        *xlog.Logger
	listener      *net.TCPListener
	closeHandler  func(c Conn)
	packetHandler func(p packet.Packet, c Conn)
	acceptHandler func(p *packet.Connect, c Conn) packet.ConnectCode
	queueCapacity int32
}

func NewGRPC(logger *xlog.Logger, queueCapacity int32) Listener {
	return &grpcListener{
		logger:        logger,
		queueCapacity: queueCapacity,
	}
}
func (l *grpcListener) Close(ctx context.Context) error {
	return l.listener.Close()
}
func (l *grpcListener) OnClose(handler func(c Conn)) {
	l.closeHandler = handler
}
func (l *grpcListener) OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *grpcListener) OnPacket(handler func(p packet.Packet, c Conn)) {
	l.packetHandler = handler
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
		c := newGRPCConn(connPacket.Identity, bidi, l.logger, l.queueCapacity)
		c.OnClose(func() {
			l.conns.Delete(connId)
			if l.closeHandler != nil {
				l.closeHandler(c)
			}
		})
		code := l.acceptHandler(connPacket, c)
		if code != packet.ConnectAccepted {
			c.ConnackCode(code, "")
			c.Close()
			continue
		}
		c.ConnackCode(packet.ConnectAccepted, connId)
		if old, ok := l.conns.Swap(connId, c); ok {
			old.CloseClode(packet.CloseDuplicateLogin)
			if old.ID().ClientID != connPacket.Identity.ClientID {
				l.logger.Error("hash collision", xlog.Str("old", old.ID().ClientID), xlog.Str("new", connPacket.Identity.ClientID), xlog.Str("connID", connId))
			}
		}
		ping := newPinger(c, time.Second*25, time.Second*3)
		ping.Start()
	}
}

type grpcConn struct {
	id  *packet.Identity
	raw grpc.BidiStreamingServer[pb.Bytes, pb.Bytes]
}

func (c *grpcConn) ID() *packet.Identity {
	return c.id
}
func (c *grpcConn) Close() error {
	return nil
}
func (c *grpcConn) WriteData(data []byte) error {
	return c.raw.Send(&pb.Bytes{Data: data})
}
func newGRPCConn(id *packet.Identity, raw grpc.BidiStreamingServer[pb.Bytes, pb.Bytes], logger *xlog.Logger, queueCapacity int32) Conn {
	g := &grpcConn{
		id:  id,
		raw: raw,
	}
	return newConn(g, logger, queueCapacity)
}
