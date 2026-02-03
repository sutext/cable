package network

import (
	"context"
	"time"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/stats"
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
	c, err := l.doConnecton(stream, ip)
	if err != nil {
		return
	}
	for {
		p, err := packet.ReadFrom(stream)
		if err != nil {
			c.CloseCode(packet.AsCloseCode(err))
			return
		}
		go c.RecvPacket(p)
	}
}
func (l *transportQUIC) doConnecton(conn *quic.Stream, ip string) (Conn, error) {
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
	c := newQUICConn(connPacket.Identity, ip, conn, l.delegate)
	code := l.delegate.OnConnect(ctx, c, connPacket)
	if code != packet.ConnectAccepted {
		return nil, code
	}
	return c, nil
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
