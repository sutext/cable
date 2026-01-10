package client

import (
	"context"
	"net"

	"golang.org/x/net/quic"
	"golang.org/x/net/websocket"
	"sutext.github.io/cable/packet"
)

type Conn interface {
	Dail() error
	Close() error
	ReadPacket() (packet.Packet, error)
	WritePacket(p packet.Packet) error
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
	addr *net.UDPAddr
	conn *net.UDPConn
	buf  []byte
}

func newUDPConn(addr string) Conn {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	return &udpConn{
		addr: udpAddr,
		buf:  make([]byte, packet.MAX_UDP),
	}
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
	n, _, err := c.conn.ReadFromUDP(c.buf)
	if err != nil {
		return nil, err
	}
	p, err := packet.Unmarshal(c.buf[:n])
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
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *udpConn) Close() error {
	return c.conn.Close()
}

type wsConn struct {
	url  string
	conn *websocket.Conn
}

func newWSConn(url string) Conn {
	return &wsConn{url: url}
}

func (c *wsConn) WritePacket(p packet.Packet) error {
	w, err := c.conn.NewFrameWriter(websocket.BinaryFrame)
	if err != nil {
		return err
	}
	return packet.WriteTo(w, p)
}
func (c *wsConn) ReadPacket() (packet.Packet, error) {
	r, err := c.conn.NewFrameReader()
	if err != nil {
		return nil, err
	}
	p, err := packet.ReadFrom(r)
	return p, nil
}

func (c *wsConn) Dail() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	ws, err := websocket.Dial(c.url, "", "")
	if err != nil {
		return err
	}
	c.conn = ws
	return nil
}

func (c *wsConn) Close() error {
	return c.conn.Close()
}

type quicConn struct {
	addr     string
	sess     *quic.Stream
	config   *quic.Config
	endpoint *quic.Endpoint
}

func newQUICConn(addr string, config *quic.Config) Conn {
	if config == nil {
		panic("quic config is nil")
	}
	return &quicConn{addr: addr, config: config}
}

func (c *quicConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.sess, p)
}

func (c *quicConn) ReadPacket() (packet.Packet, error) {
	return packet.ReadFrom(c.sess)
}

func (c *quicConn) Dail() error {
	pkgConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	e, err := quic.NewEndpoint(pkgConn, c.config)
	if err != nil {
		return err
	}
	conn, err := e.Dial(context.Background(), "udp", c.addr, c.config)
	if err != nil {
		return err
	}
	sess, err := conn.NewStream(context.Background())
	if err != nil {
		return err
	}
	c.sess = sess
	c.endpoint = e
	return nil
}

func (c *quicConn) Close() error {
	return c.endpoint.Close(context.Background())
}
