package listener

import (
	"net"
	"sync"

	"sutext.github.io/cable/packet"
)

type udpListener struct {
	connMap       sync.Map
	packetHandler func(p packet.Packet, id *packet.Identity)
	acceptHandler func(p *packet.Connect, c Conn) packet.ConnectCode
}

func NewUDP() Listener {
	return &udpListener{}
}
func (l *udpListener) OnAccept(handler func(*packet.Connect, Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}

func (l *udpListener) OnPacket(handler func(p packet.Packet, id *packet.Identity)) {
	l.packetHandler = handler
}
func (l *udpListener) Listen(address string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	buf := make([]byte, packet.MAX_UDP)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		go l.handleConn(conn, addr, p)
	}
}
func (l *udpListener) handleConn(conn *net.UDPConn, addr *net.UDPAddr, p packet.Packet) {
	if p.Type() != packet.CONNECT {
		if c, loaded := l.connMap.Load(addr.String()); loaded {
			l.packetHandler(p, c.(Conn).ID())
		}
		return
	}
	connPacket := p.(*packet.Connect)
	c := newUDPConn(connPacket.Identity, conn, addr)
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.WritePacket(packet.NewConnack(code))
		return
	}
	c.WritePacket(packet.NewConnack(packet.ConnectAccepted))
	l.connMap.Store(addr.String(), c)
}

// Below is the udpConn implementation
type udpConn struct {
	id   *packet.Identity
	raw  *net.UDPConn
	addr *net.UDPAddr
}

func newUDPConn(id *packet.Identity, raw *net.UDPConn, addr *net.UDPAddr) *udpConn {
	return &udpConn{
		id:   id,
		raw:  raw,
		addr: addr,
	}
}

func (c *udpConn) ID() *packet.Identity {
	return c.id
}
func (c *udpConn) Close() error {
	return nil
}
func (c *udpConn) WritePacket(p packet.Packet) error {
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	_, err = c.raw.WriteToUDP(data, c.addr)
	return err
}
