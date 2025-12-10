package client

import (
	"net"
	"sync"

	"sutext.github.io/cable/packet"
)

type Conn interface {
	Dail() error
	Close() error
	ReadPacket() (packet.Packet, error)
	WritePacket(p packet.Packet) error
}

func NewConn(network Network, addr string) Conn {
	switch network {
	case NewworkTCP:
		return newTCPConn(addr)
	case NetworkUDP:
		return newUDPConn(addr)
	default:
		panic("unknown network")
	}
}

type tcpConn struct {
	addr *net.TCPAddr
	conn *net.TCPConn
}

func newTCPConn(addr string) Conn {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	return &tcpConn{addr: tcpAddr}
}

func (c *tcpConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.conn, p)
}

func (c *tcpConn) ReadPacket() (packet.Packet, error) {
	return packet.ReadFrom(c.conn)
}

func (c *tcpConn) Dail() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	conn, err := net.DialTCP("tcp", nil, c.addr)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *tcpConn) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

type udpConn struct {
	addr    *net.UDPAddr
	conn    *net.UDPConn
	bufPool sync.Pool
}

func newUDPConn(addr string) Conn {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	return &udpConn{
		addr: udpAddr,
		bufPool: sync.Pool{
			New: func() any {
				return make([]byte, packet.MAX_UDP)
			},
		}}
}

func (c *udpConn) WritePacket(p packet.Packet) error {
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > packet.MAX_UDP {
		return packet.ErrPacketSizeTooLarge
	}
	_, err = c.conn.WriteToUDP(data, c.addr)
	return err
}

func (c *udpConn) ReadPacket() (packet.Packet, error) {
	buf := c.bufPool.Get().([]byte)
	defer c.bufPool.Put(buf[:0])
	n, _, err := c.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	p, err := packet.Unmarshal(buf[:n])
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (c *udpConn) Dail() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *udpConn) Close() error {
	return c.conn.Close()
}
