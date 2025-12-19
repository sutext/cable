package listener

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type udpListener struct {
	conns         safe.Map[string, *Conn]
	logger        *xlog.Logger
	listener      *net.UDPConn
	closeHandler  func(c *Conn)
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(p *packet.Connect, c *Conn) packet.ConnectCode
}

func NewUDP() Listener {
	return &udpListener{
		logger: xlog.With("GROUP", "SERVER"),
	}
}
func (l *udpListener) OnClose(handler func(*Conn)) {
	l.closeHandler = handler
}
func (l *udpListener) OnAccept(handler func(*packet.Connect, *Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *udpListener) OnPacket(handler func(p packet.Packet, c *Conn)) {
	l.packetHandler = handler
}
func (l *udpListener) Close(ctx context.Context) error {
	return l.listener.Close()
}
func (l *udpListener) Listen(address string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	l.listener = listener
	defer listener.Close()
	buf := make([]byte, packet.MAX_UDP)
	for {
		n, addr, err := listener.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		go l.handleConn(listener, addr, p)
	}
}

func (l *udpListener) handleConn(conn *net.UDPConn, addr *net.UDPAddr, p packet.Packet) {
	if p.Type() != packet.CONNECT {
		connID, ok := p.Get(packet.PropertyConnID)
		if !ok {
			l.logger.Warn("unknown udp packet", xlog.Str("packet", p.String()))
			return
		}
		c, loaded := l.conns.Get(connID)
		if !loaded {
			l.logger.Warn("unknown udp connID", xlog.Str("connID", connID))
			return
		}
		go l.packetHandler(p, c)
		return
	}
	connPacket := p.(*packet.Connect)
	connId := genConnId(connPacket.Identity.ClientID)
	uc := newUDPConn(connPacket.Identity, conn, addr)
	c := newConn(uc)
	uc.closeHandler = func() {
		l.conns.Delete(connId)
		if l.closeHandler != nil {
			l.closeHandler(c)
		}
	}
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ClosePacket(packet.NewConnack(code))
		return
	}
	ack := packet.NewConnack(packet.ConnectAccepted)
	ack.Set(packet.PropertyConnID, connId)
	c.SendPacket(context.Background(), ack)
	if old, ok := l.conns.Swap(connId, c); ok {
		old.ClosePacket(packet.NewClose(packet.CloseDuplicateLogin))
		if old.ID().ClientID != connPacket.Identity.ClientID {
			l.logger.Error("hash collision", xlog.Str("old", old.ID().ClientID), xlog.Str("new", connPacket.Identity.ClientID), xlog.Str("connID", connId))
		}
	}
}

type udpConn struct {
	id           *packet.Identity
	raw          *net.UDPConn
	addr         atomic.Pointer[net.UDPAddr]
	closed       atomic.Bool
	closeHandler func()
}

func (c *udpConn) ID() *packet.Identity {
	return c.id
}
func (c *udpConn) close() error {
	if c.closed.CompareAndSwap(false, true) {
		err := c.raw.Close()
		if c.closeHandler != nil {
			c.closeHandler()
		}
		return err
	}
	return nil
}
func (c *udpConn) isClosed() bool {
	return c.closed.Load()
}
func (c *udpConn) writePacket(ctx context.Context, p packet.Packet, jump bool) error {
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	c.raw.SetWriteDeadline(time.Now().Add(time.Second * 5))
	_, err = c.raw.WriteToUDP(data, c.addr.Load())
	return err
}
func newUDPConn(id *packet.Identity, raw *net.UDPConn, addr *net.UDPAddr) *udpConn {
	c := &udpConn{
		id:  id,
		raw: raw,
	}
	c.addr.Store(addr)
	return c
}
