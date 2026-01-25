package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type transportWebSocket struct {
	logger     *xlog.Logger
	tlsConfig  *tls.Config
	httpServer *http.Server
	delegate   delegate
}

func NewWS(delegate delegate) Transport {
	return &transportWebSocket{
		logger:   delegate.Logger(),
		delegate: delegate,
	}
}
func NewWSS(config *tls.Config, delegate delegate) Transport {
	assertTLS(config)
	return &transportWebSocket{
		logger:    delegate.Logger(),
		tlsConfig: config,
		delegate:  delegate,
	}
}
func (l *transportWebSocket) Close(ctx context.Context) error {
	return l.httpServer.Close()
}
func (l *transportWebSocket) Listen(address string) error {
	l.httpServer = &http.Server{
		Addr: address,
		Handler: websocket.Server{
			Handler: func(conn *websocket.Conn) {
				conn.PayloadType = websocket.BinaryFrame
				l.handleConn(conn)
			},
			Handshake: func(c *websocket.Config, r *http.Request) (err error) {
				if c.Protocol[0] != "cable" {
					return fmt.Errorf("invalid protocol")
				}
				return nil
			},
		},
		TLSConfig: l.tlsConfig,
	}
	if l.tlsConfig != nil {
		return l.httpServer.ListenAndServeTLS("", "")
	}
	return l.httpServer.ListenAndServe()
}
func (l *transportWebSocket) handleConn(conn *websocket.Conn) {
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
	realIp := getRealIP(conn.Request())
	c := newWSConn(connPacket.Identity, realIp, conn, l.delegate)
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

type wsConn struct {
	id  *packet.Identity
	ip  string
	raw *websocket.Conn
}

func (c *wsConn) ID() *packet.Identity {
	return c.id
}
func (c *wsConn) IP() string {
	return c.ip
}
func (c *wsConn) Close() error {
	return c.raw.Close()
}

func (c *wsConn) WriteData(data []byte) error {
	_, err := c.raw.Write(data)
	return err
}
func newWSConn(id *packet.Identity, ip string, raw *websocket.Conn, delegate delegate) Conn {
	t := &wsConn{
		id:  id,
		ip:  ip,
		raw: raw,
	}
	return newConn(t, delegate)
}
