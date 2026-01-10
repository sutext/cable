package network

import (
	"context"
	"time"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type transportQUIC struct {
	logger        *xlog.Logger
	config        *quic.Config
	endpoint      *quic.Endpoint
	closeHandler  func(c Conn)
	packetHandler func(p packet.Packet, c Conn)
	acceptHandler func(p *packet.Connect, c Conn) packet.ConnectCode
	queueCapacity int32
}

func NewQUIC(logger *xlog.Logger, queueCapacity int32, config *quic.Config) Transport {
	if config == nil {
		panic("quic config is nil")
	}
	return &transportQUIC{
		logger:        logger,
		queueCapacity: queueCapacity,
		config:        config,
	}
}
func (l *transportQUIC) Close(ctx context.Context) error {
	return l.endpoint.Close(ctx)
}
func (l *transportQUIC) OnClose(handler func(c Conn)) {
	l.closeHandler = handler
}
func (l *transportQUIC) OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *transportQUIC) OnPacket(handler func(p packet.Packet, c Conn)) {
	l.packetHandler = handler
}

func (l *transportQUIC) Listen(address string) error {
	endpoint, err := quic.Listen("udp", address, l.config)
	if err != nil {
		return nil
	}
	defer endpoint.Close(context.Background())
	for {
		conn, err := endpoint.Accept(context.Background())
		if err != nil {
			return err
		}
		ss, err := conn.AcceptStream(context.Background())
		if err != nil {
			return err
		}
		go l.handleConn(ss, string(conn.RemoteAddr().Addr().String()))
	}
}
func (l *transportQUIC) handleConn(stream *quic.Stream, ip string) {
	timer := time.AfterFunc(time.Second*10, func() {
		l.logger.Warn("waite conn packet timeout")
		packet.WriteTo(stream, packet.NewClose(packet.CloseAuthTimeout))
		stream.Close()
	})
	p, err := packet.ReadFrom(stream)
	timer.Stop()
	if err != nil {
		l.logger.Error("failed to read packet", xlog.Err(err))
		packet.WriteTo(stream, packet.NewClose(packet.AsCloseCode(err)))
		stream.Close()
		return
	}
	if p.Type() != packet.CONNECT {
		l.logger.Error("first packet is not connect packet", xlog.Str("packetType", p.Type().String()))
		packet.WriteTo(stream, packet.NewClose(packet.AsCloseCode(err)))
		stream.Close()
		return
	}
	connPacket := p.(*packet.Connect)
	c := newQUICConn(connPacket.Identity, ip, stream, l.logger, l.queueCapacity)
	c.OnClose(func() {
		l.closeHandler(c)
	})
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ConnackCode(code, "")
		c.Close()
		return
	}
	c.ConnackCode(packet.ConnectAccepted, "")
	for {
		p, err := packet.ReadFrom(stream)
		if err != nil {
			c.CloseClode(packet.AsCloseCode(err))
			return
		}
		go l.packetHandler(p, c)
	}
}

type quicConn struct {
	id  *packet.Identity
	ip  string
	raw *quic.Stream
}

func (c *quicConn) ID() *packet.Identity {
	return c.id
}
func (c *quicConn) IP() string {
	return c.ip
}
func (c *quicConn) Close() error {
	return c.raw.Close()
}

func (c *quicConn) WriteData(data []byte) error {
	_, err := c.raw.Write(data)
	return err
}
func newQUICConn(id *packet.Identity, ip string, raw *quic.Stream, logger *xlog.Logger, queueCapacity int32) Conn {
	t := &quicConn{
		id:  id,
		ip:  ip,
		raw: raw,
	}
	return newConn(t, logger, queueCapacity)
}
