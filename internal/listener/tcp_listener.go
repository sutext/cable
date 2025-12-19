package listener

import (
	"context"
	"net"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type tcpListener struct {
	logger        *xlog.Logger
	listener      *net.TCPListener
	closeHandler  func(c *Conn)
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(*packet.Connect, *Conn) packet.ConnectCode
	queueCapacity int32
}

func NewTCP(logger *xlog.Logger, queueCapacity int32) Listener {
	return &tcpListener{
		logger:        logger,
		queueCapacity: queueCapacity,
	}
}
func (l *tcpListener) OnClose(handler func(c *Conn)) {
	l.closeHandler = handler
}
func (l *tcpListener) OnAccept(handler func(*packet.Connect, *Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *tcpListener) OnPacket(handler func(p packet.Packet, c *Conn)) {
	l.packetHandler = handler
}
func (l *tcpListener) Close(ctx context.Context) error {
	return l.listener.Close()
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
	l.listener = listener
	defer listener.Close()
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return err
		}
		go l.handleConn(conn)
	}
}
func (l *tcpListener) handleConn(conn *net.TCPConn) {
	timer := time.AfterFunc(time.Second*10, func() {
		l.logger.Warn("waite conn packet timeout")
		packet.WriteTo(conn, packet.NewClose(packet.CloseAuthenticationTimeout))
		conn.Close()
	})
	p, err := packet.ReadFrom(conn)
	timer.Stop()
	if err != nil {
		l.logger.Error("failed to read packet", xlog.Err(err))
		packet.WriteTo(conn, packet.NewClose(packet.AsCloseCode(err)))
		conn.Close()
		return
	}
	if p.Type() != packet.CONNECT {
		l.logger.Error("first packet is not connect packet", xlog.Str("packetType", p.Type().String()))
		packet.WriteTo(conn, packet.NewClose(packet.AsCloseCode(err)))
		conn.Close()
		return
	}
	connPacket := p.(*packet.Connect)
	c := newTCPConn(connPacket.Identity, conn, l.logger, l.queueCapacity)
	c.closeHandler = func() {
		l.closeHandler(c)
	}
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ConnackCode(code, "")
		c.Close()
		return
	}
	c.ConnackCode(packet.ConnectAccepted, "")
	for {
		p, err := packet.ReadFrom(conn)
		if err != nil {
			c.CloseClode(packet.AsCloseCode(err))
			return
		}
		go l.packetHandler(p, c)
	}
}

type tcpConn struct {
	raw      *net.TCPConn
	identity *packet.Identity
}

func (c *tcpConn) id() *packet.Identity {
	return c.identity
}
func (c *tcpConn) close() error {
	return c.raw.Close()
}

func (c *tcpConn) writeData(data []byte) error {
	_, err := c.raw.Write(data)
	return err
}
func newTCPConn(id *packet.Identity, raw *net.TCPConn, logger *xlog.Logger, queueCapacity int32) *Conn {
	t := &tcpConn{
		identity: id,
		raw:      raw,
	}
	return newConn(t, logger, queueCapacity)
}
