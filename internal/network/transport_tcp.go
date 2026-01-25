package network

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type transportTCP struct {
	logger    *xlog.Logger
	listener  net.Listener
	tlsConfig *tls.Config
	delegate  delegate
}

func NewTCP(delegate delegate) Transport {
	return &transportTCP{
		logger:   delegate.Logger(),
		delegate: delegate,
	}
}
func NewTLS(config *tls.Config, delegate delegate) Transport {
	assertTLS(config)
	return &transportTCP{
		logger:    delegate.Logger(),
		tlsConfig: config,
		delegate:  delegate,
	}
}
func (l *transportTCP) Close(ctx context.Context) error {
	return l.listener.Close()
}

func (l *transportTCP) Listen(address string) error {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	if l.tlsConfig != nil {
		l.listener = tls.NewListener(listener, l.tlsConfig)
	} else {
		l.listener = listener
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go l.handleConn(conn)
	}
}
func (l *transportTCP) handleConn(conn net.Conn) {
	timer := time.AfterFunc(time.Second*10, func() {
		l.logger.Warn("waite conn packet timeout")
		packet.WriteTo(conn, packet.NewClose(packet.CloseAuthTimeout))
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
	c := newTCPConn(connPacket.Identity, getAddrIp(conn.RemoteAddr().String()), conn, l.delegate)
	if err := l.delegate.OnConnect(c, connPacket); err != nil {
		l.logger.Error("connect error", xlog.Err(err))
		return
	}
	for {
		p, err := packet.ReadFrom(conn)
		if err != nil {
			c.CloseCode(packet.AsCloseCode(err))
			return
		}
		go l.delegate.OnPacket(c, p)
	}
}

type tcpConn struct {
	id  *packet.Identity
	ip  string
	raw net.Conn
}

func (c *tcpConn) ID() *packet.Identity {
	return c.id
}
func (c *tcpConn) IP() string {
	return c.ip
}
func (c *tcpConn) Close() error {
	return c.raw.Close()
}

func (c *tcpConn) WriteData(data []byte) error {
	_, err := c.raw.Write(data)
	return err
}
func newTCPConn(id *packet.Identity, ip string, raw net.Conn, delegate delegate) Conn {
	t := &tcpConn{
		id:  id,
		ip:  ip,
		raw: raw,
	}
	return newConn(t, delegate)
}
