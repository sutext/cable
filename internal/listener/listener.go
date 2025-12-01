package listener

import (
	"context"
	"net"
	"sync/atomic"

	"sutext.github.io/cable/packet"
)

type Listener interface {
	Listen(addr string) error
	Close(ctx context.Context) error
	OnPacket(handler func(p packet.Packet, c *Conn))
	OnAccept(handler func(p *packet.Connect, c *Conn) packet.ConnectCode)
}

type conn interface {
	ID() *packet.Identity
	close() error
	writePacket(p packet.Packet) error
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
func (c *udpConn) writePacket(p packet.Packet) error {
	p.PropSet(packet.PropertyUDPConnID, c.connID)
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
