package listener

import (
	"net"
	"time"

	"sutext.github.io/cable/packet"
)

type tcpListener struct {
	packetHandler func(p packet.Packet, id *packet.Identity)
	acceptHandler func(*packet.Connect, Conn) packet.ConnectCode
}

func NewTCP() Listener {
	return &tcpListener{}
}
func (l *tcpListener) OnAccept(handler func(*packet.Connect, Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *tcpListener) OnPacket(handler func(p packet.Packet, id *packet.Identity)) {
	l.packetHandler = handler
}
func (l *tcpListener) Listen(address string) error {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return err
		}
		go l.handleConn(conn)
	}
}

type connPacketCloseCode struct {
	connPacket *packet.Connect
	closeCode  packet.CloseCode
}

func (l *tcpListener) handleConn(conn *net.TCPConn) {
	defer conn.Close()
	ch := make(chan connPacketCloseCode)
	defer close(ch)
	go l.readConnect(conn, ch)
	select {
	case r := <-ch:
		if r.connPacket == nil {
			packet.WriteTo(conn, packet.NewClose(r.closeCode))
			return
		}
		c := newTCPConn(r.connPacket.Identity, conn)
		code := l.acceptHandler(r.connPacket, c)
		if code != packet.ConnectAccepted {
			c.WritePacket(packet.NewConnack(code))
			return
		}
		c.WritePacket(packet.NewConnack(packet.ConnectAccepted))
		for {
			p, err := packet.ReadFrom(conn)
			if err != nil {
				code := closeCodeOf(err)
				c.WritePacket(packet.NewClose(code))
				return
			}
			l.packetHandler(p, c.id)
		}
	case <-time.After(time.Second * 10):
		packet.WriteTo(conn, packet.NewClose(packet.CloseAuthenticationTimeout))
		return
	}
}

func (l *tcpListener) readConnect(conn *net.TCPConn, ch chan<- connPacketCloseCode) {
	p, err := packet.ReadFrom(conn)
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

// Below is the tcpConn implementation of Conn interface
type tcpConn struct {
	id  *packet.Identity
	raw *net.TCPConn
}

func newTCPConn(id *packet.Identity, raw *net.TCPConn) *tcpConn {
	return &tcpConn{
		id:  id,
		raw: raw,
	}
}
func (c *tcpConn) ID() *packet.Identity {
	return c.id
}
func (c *tcpConn) Close() error {
	return c.raw.Close()
}
func (c *tcpConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.raw, p)
}
