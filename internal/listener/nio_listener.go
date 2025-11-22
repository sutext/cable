package listener

import (
	"context"
	"net"
	"time"

	"github.com/cloudwego/netpoll"
	"sutext.github.io/cable/packet"
)

type contextKey struct{}

var identityKey = contextKey{}

type nioListener struct {
	eventLoop     netpoll.EventLoop
	packetHandler func(p packet.Packet, id *packet.Identity)
	acceptHandler func(*packet.Connect, Conn) packet.ConnectCode
}

func NewNIO() Listener {
	return &nioListener{}
}
func (l *nioListener) OnAccept(handler func(*packet.Connect, Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *nioListener) OnPacket(handler func(p packet.Packet, id *packet.Identity)) {
	l.packetHandler = handler
}
func (l *nioListener) Listen(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	eventLoop, err := netpoll.NewEventLoop(l.onRequest,
		netpoll.WithOnPrepare(l.onPrepare),
		netpoll.WithOnConnect(l.onConnect),
		netpoll.WithOnDisconnect(l.onDisconnect),
	)
	if err != nil {
		return err
	}
	l.eventLoop = eventLoop
	return eventLoop.Serve(ln)
}

func (l *nioListener) onPrepare(conn netpoll.Connection) context.Context {
	return context.Background()
}
func (l *nioListener) onConnect(ctx context.Context, conn netpoll.Connection) context.Context {
	ch := make(chan connPacketCloseCode)
	defer close(ch)
	go l.readConnect(conn, ch)
	select {
	case r := <-ch:
		if r.connPacket == nil {
			packet.WriteTo(conn, packet.NewClose(r.closeCode))
			conn.Close()
			return ctx
		}
		c := newNIOConn(r.connPacket.Identity, conn)
		code := l.acceptHandler(r.connPacket, c)
		if code != packet.ConnectAccepted {
			c.WritePacket(packet.NewConnack(code))
			c.Close()
			return ctx
		}
		c.WritePacket(packet.NewConnack(packet.ConnectAccepted))
		return context.WithValue(ctx, identityKey, c.id)
	case <-time.After(time.Second * 10):
		packet.WriteTo(conn, packet.NewClose(packet.CloseAuthenticationTimeout))
		conn.Close()
		return ctx
	}
}

func (l *nioListener) onRequest(ctx context.Context, conn netpoll.Connection) error {
	id := ctx.Value(identityKey).(*packet.Identity)
	reader := netpoll.NewIOReader(conn.Reader())
	pkt, err := packet.ReadFrom(reader)
	if err != nil {
		code := closeCodeOf(err)
		packet.WriteTo(conn, packet.NewClose(code))
		conn.Close()
		return err
	}
	l.packetHandler(pkt, id)
	return nil
}
func (l *nioListener) onDisconnect(ctx context.Context, conn netpoll.Connection) {
	// TODO: id, ok := ctx.Value(identityKey).(*packet.Identity)

}

func (l *nioListener) readConnect(conn netpoll.Connection, ch chan<- connPacketCloseCode) {
	p, err := packet.ReadFrom(netpoll.NewIOReader(conn.Reader()))
	if err != nil {
		ch <- connPacketCloseCode{
			closeCode: closeCodeOf(err),
		}
		return
	}
	if p.Type() != packet.CONNECT {
		ch <- connPacketCloseCode{
			closeCode: packet.CloseInvalidPacket,
		}
		return
	}
	ch <- connPacketCloseCode{
		connPacket: p.(*packet.Connect),
	}
}

// Below is the nioConn implementation of Conn interface
type nioConn struct {
	id  *packet.Identity
	raw netpoll.Connection
}

func newNIOConn(id *packet.Identity, raw netpoll.Connection) *nioConn {
	return &nioConn{
		id:  id,
		raw: raw,
	}
}
func (c *nioConn) ID() *packet.Identity {
	return c.id
}
func (c *nioConn) Close() error {
	return c.raw.Close()
}
func (c *nioConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.raw, p)
}
