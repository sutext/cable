package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/stats"
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
func (l *transportWebSocket) doConnecton(conn *websocket.Conn) (Conn, error) {
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
	realIp := getRealIP(conn.Request())
	c := newWSConn(connPacket.Identity, realIp, conn, l.delegate)
	code := l.delegate.OnConnect(ctx, c, connPacket)
	if code != packet.ConnectAccepted {
		return nil, code
	}
	return c, nil
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
