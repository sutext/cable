package network

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/stats"
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
	c, err := l.doConnecton(conn)
	if err != nil {
		return
	}
	for {
		p, err := packet.ReadFrom(conn)
		if err != nil {
			c.CloseCode(packet.AsCloseCode(err))
			return
		}
		go c.RecvPacket(p)
	}
}
func (l *transportTCP) doConnecton(conn net.Conn) (Conn, error) {
	ctx := context.Background()
	var err error
	handler := l.delegate.StatsHandler()
	if handler != nil {
		beginTime := time.Now()
		ctx = handler.ConnectBegin(ctx, &stats.ConnBegin{
			BeginTime: beginTime,
		})
		defer handler.ConnectEnd(ctx, &stats.ConnEnd{
			BeginTime: beginTime,
			EndTime:   time.Now(),
			Error:     err,
		})
	}
	var connPacket *packet.Connect
	connPacket, err = waitConnPacket(ctx, conn, l.delegate)
	if err != nil {
		return nil, err
	}
	c := newTCPConn(connPacket.Identity, getAddrIp(conn.RemoteAddr().String()), conn, l.delegate)
	code := l.delegate.OnConnect(ctx, c, connPacket)
	if code != packet.ConnectAccepted {
		return nil, code
	}
	return c, nil
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
