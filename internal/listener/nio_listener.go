package listener

import (
	"context"
	"net"
	"time"

	"github.com/cloudwego/netpoll"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type contextKey struct{}

var connectionKey = contextKey{}

type nioListener struct {
	logger        *xlog.Logger
	eventLoop     netpoll.EventLoop
	closeHandler  func(c *Conn)
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(*packet.Connect, *Conn) packet.ConnectCode
}

func NewNIO() Listener {
	return &nioListener{
		logger: xlog.With("GROUP", "SERVER"),
	}
}
func (l *nioListener) OnClose(handler func(c *Conn)) {
	l.closeHandler = handler
}
func (l *nioListener) OnAccept(handler func(*packet.Connect, *Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *nioListener) OnPacket(handler func(p packet.Packet, c *Conn)) {
	l.packetHandler = handler
}
func (l *nioListener) Close(ctx context.Context) error {
	return l.eventLoop.Shutdown(ctx)
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
	timer := time.AfterFunc(time.Second*10, func() {
		l.logger.Warn("waite conn packet timeout")
		conn.Close()
	})
	p, err := packet.ReadFrom(conn)
	timer.Stop()
	if err != nil {
		l.logger.Error("failed to read packet", xlog.Err(err))
		conn.Close()
		return ctx
	}
	if p.Type() != packet.CONNECT {
		l.logger.Error("first packet is not connect packet", xlog.String("packetType", p.Type().String()))
		conn.Close()
		return ctx
	}
	connPacket := p.(*packet.Connect)
	c := newNIOConn(connPacket.Identity, conn)
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ClosePacket(packet.NewConnack(code))
		return ctx
	}
	c.SendPacket(packet.NewConnack(packet.ConnectAccepted))
	return context.WithValue(ctx, connectionKey, c)
}

func (l *nioListener) onRequest(ctx context.Context, conn netpoll.Connection) error {
	c := ctx.Value(connectionKey).(*Conn)
	reader := netpoll.NewIOReader(conn.Reader())
	p, err := packet.ReadFrom(reader)
	if err != nil {
		c.ClosePacket(packet.NewClose(packet.AsCloseCode(err)))
		return err
	}
	l.packetHandler(p, c)
	return nil
}
func (l *nioListener) onDisconnect(ctx context.Context, conn netpoll.Connection) {
	c, ok := ctx.Value(connectionKey).(*Conn)
	if ok && l.closeHandler != nil {
		l.closeHandler(c)
	}
}

type nioConn struct {
	id  *packet.Identity
	raw netpoll.Connection
}

func (c *nioConn) ID() *packet.Identity {
	return c.id
}
func (c *nioConn) close() error {
	return c.raw.Close()
}
func (c *nioConn) isClosed() bool {
	return !c.raw.IsActive()
}
func (c *nioConn) writePacket(p packet.Packet, jump bool) error {
	w := c.raw.Writer()
	err := packet.WriteTo(netpoll.NewIOWriter(w), p)
	if err != nil {
		return err
	}
	return w.Flush()
}

func newNIOConn(id *packet.Identity, raw netpoll.Connection) *Conn {
	return newConn(&nioConn{
		id:  id,
		raw: raw,
	})
}
