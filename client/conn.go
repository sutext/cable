// Package client provides a client implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like automatic reconnection, heartbeat detection, and request-response mechanism.
package client

import (
	"context"
	"net"

	"golang.org/x/net/quic"
	"golang.org/x/net/websocket"
	"sutext.github.io/cable/packet"
)

// Conn defines the interface for network connections that handle packet transmission.
type Conn interface {
	// Dail establishes a connection to the remote server.
	Dail() error
	// Close closes the connection and releases resources.
	Close() error
	// ReadPacket reads a packet from the connection.
	ReadPacket() (packet.Packet, error)
	// WritePacket writes a packet to the connection.
	WritePacket(p packet.Packet) error
}

// tcpConn implements the Conn interface for TCP connections.
type tcpConn struct {
	addr *net.TCPAddr // TCP address of the remote server
	conn *net.TCPConn // Underlying TCP connection
}

// newTCPConn creates a new TCP connection instance.
func newTCPConn(addr string) Conn {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	return &tcpConn{addr: tcpAddr}
}

// WritePacket writes a packet to the TCP connection.
func (c *tcpConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.conn, p)
}

// ReadPacket reads a packet from the TCP connection.
func (c *tcpConn) ReadPacket() (packet.Packet, error) {
	return packet.ReadFrom(c.conn)
}

// Dail establishes a TCP connection to the remote server.
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

// Close closes the TCP connection and releases resources.
func (c *tcpConn) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// udpConn implements the Conn interface for UDP connections.
type udpConn struct {
	addr *net.UDPAddr // UDP address of the remote server
	conn *net.UDPConn // Underlying UDP connection
	buf  []byte       // Buffer for reading packets
}

// newUDPConn creates a new UDP connection instance.
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

// WritePacket writes a packet to the UDP connection.
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

// ReadPacket reads a packet from the UDP connection.
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

// Dail initializes a UDP connection for sending and receiving packets.
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

// Close closes the UDP connection.
func (c *udpConn) Close() error {
	return c.conn.Close()
}

// wsConn implements the Conn interface for WebSocket connections.
type wsConn struct {
	url  string          // WebSocket URL of the remote server
	conn *websocket.Conn // Underlying WebSocket connection
}

// newWSConn creates a new WebSocket connection instance.
func newWSConn(addr string) Conn {
	return &wsConn{url: addr}
}

// WritePacket writes a packet to the WebSocket connection.
func (c *wsConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.conn, p)
}

// ReadPacket reads a packet from the WebSocket connection.
func (c *wsConn) ReadPacket() (packet.Packet, error) {
	return packet.ReadFrom(c.conn)
}

// Dail establishes a WebSocket connection to the remote server.
func (c *wsConn) Dail() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	ws, err := websocket.Dial(c.url, "cable", "https://localhost")
	if err != nil {
		return err
	}
	ws.PayloadType = websocket.BinaryFrame
	c.conn = ws
	return nil
}

// Close closes the WebSocket connection.
func (c *wsConn) Close() error {
	return c.conn.Close()
}

// quicConn implements the Conn interface for QUIC connections.
type quicConn struct {
	addr     string          // QUIC address of the remote server
	stream   *quic.Stream    // QUIC stream for reading/writing packets
	config   *quic.Config    // QUIC configuration
	endpoint *quic.Endpoint  // QUIC endpoint
}

// newQUICConn creates a new QUIC connection instance.
func newQUICConn(addr string, config *quic.Config) Conn {
	if config == nil {
		panic("quic config is nil")
	}
	return &quicConn{addr: addr, config: config}
}

// WritePacket writes a packet to the QUIC stream.
func (c *quicConn) WritePacket(p packet.Packet) error {
	return packet.WriteTo(c.stream, p)
}

// ReadPacket reads a packet from the QUIC stream.
func (c *quicConn) ReadPacket() (packet.Packet, error) {
	return packet.ReadFrom(c.stream)
}

// Dail establishes a QUIC connection to the remote server.
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
	s, err := conn.NewStream(context.Background())
	if err != nil {
		return err
	}
	c.stream = s
	c.endpoint = e
	return nil
}

// Close closes the QUIC endpoint and releases resources.
func (c *quicConn) Close() error {
	return c.endpoint.Close(context.Background())
}
