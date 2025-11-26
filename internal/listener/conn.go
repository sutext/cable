package listener

import (
	"net"

	"sutext.github.io/cable/packet"
)

type conn interface {
	ID() *packet.Identity
	close() error
	writePacket(p packet.Packet) error
}

type udpConn struct {
	id   *packet.Identity
	raw  *net.UDPConn
	addr *net.UDPAddr
}

func (c *udpConn) ID() *packet.Identity {
	return c.id
}
func (c *udpConn) close() error {
	return nil
}
func (c *udpConn) writePacket(p packet.Packet) error {
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	_, err = c.raw.WriteToUDP(data, c.addr)
	return err
}

func newUDPConn(id *packet.Identity, raw *net.UDPConn, addr *net.UDPAddr) *Conn {
	return newConn(&udpConn{
		id:   id,
		raw:  raw,
		addr: addr,
	})
}

type tcpConn struct {
	id  *packet.Identity
	raw *net.TCPConn
}

func (c *tcpConn) ID() *packet.Identity {
	return c.id
}
func (c *tcpConn) close() error {
	return c.raw.Close()
}
func (c *tcpConn) writePacket(p packet.Packet) error {
	return packet.WriteTo(c.raw, p)
}
func newTCPConn(id *packet.Identity, raw *net.TCPConn) *Conn {
	return newConn(&tcpConn{
		id:  id,
		raw: raw,
	})
}
