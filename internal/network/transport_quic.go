package network

import (
	"context"
	"time"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type transportQUIC struct {
	logger   *xlog.Logger
	config   *quic.Config
	endpoint *quic.Endpoint
	delegate delegate
}

func NewQUIC(config *quic.Config, delegate delegate) Transport {
	if config == nil {
		panic("quic config is nil")
	}
	assertTLS(config.TLSConfig)
	return &transportQUIC{
		logger: delegate.Logger(),
		config: config,
	}
}
func (l *transportQUIC) Close(ctx context.Context) error {
	return l.endpoint.Close(ctx)
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
	c := newQUICConn(connPacket.Identity, ip, stream, l.delegate)
	if err := l.delegate.OnConnect(c, connPacket); err != nil {
		l.logger.Error("connect error", xlog.Err(err))
		return
	}
	for {
		p, err := packet.ReadFrom(stream)
		if err != nil {
			c.CloseCode(packet.AsCloseCode(err))
			return
		}
		go l.delegate.OnPacket(c, p)
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
func newQUICConn(id *packet.Identity, ip string, raw *quic.Stream, delegate delegate) Conn {
	t := &quicConn{
		id:  id,
		ip:  ip,
		raw: raw,
	}
	return newConn(t, delegate)
}
