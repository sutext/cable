package listener

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net"
	"sync/atomic"

	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type udpListener struct {
	logger        *xlog.Logger
	connMap       safe.Map[string, *Conn]
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
			l.logger.Warn("unknown udp packet", xlog.String("packet", p.String()))
			return
		}
		c, loaded := l.connMap.Get(connID)
		if !loaded {
			l.logger.Warn("unknown udp connID", xlog.String("connID", connID))
			return
		}
		go l.packetHandler(p, c)
		return
	}
	connPacket := p.(*packet.Connect)
	connID := md5Hash(connPacket.Identity.ClientID)
	c := newUDPConn(connPacket.Identity, conn, addr, connID)
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ClosePacket(packet.NewConnack(code))
		return
	}
	c.SendPacket(packet.NewConnack(packet.ConnectAccepted))
	l.connMap.Set(addr.String(), c)
}
func md5Hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

type udpConn struct {
	id     *packet.Identity
	raw    *net.UDPConn
	addr   atomic.Pointer[net.UDPAddr]
	connID string
}

func (c *udpConn) ID() *packet.Identity {
	return c.id
}
func (c *udpConn) close() error {
	return nil
}
func (c *udpConn) isClosed() bool {
	return false
}
func (c *udpConn) writePacket(p packet.Packet, jump bool) error {
	p.Set(packet.PropertyConnID, c.connID)
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	_, err = c.raw.WriteToUDP(data, c.addr.Load())
	return err
}

func newUDPConn(id *packet.Identity, raw *net.UDPConn, addr *net.UDPAddr, connID string) *Conn {
	c := &udpConn{
		id:     id,
		raw:    raw,
		connID: connID,
	}
	c.addr.Store(addr)
	return newConn(c)
}
