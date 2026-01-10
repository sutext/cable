package network

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type transportWebSocket struct {
	logger        *xlog.Logger
	httpServer    *http.Server
	closeHandler  func(c Conn)
	packetHandler func(p packet.Packet, c Conn)
	acceptHandler func(p *packet.Connect, c Conn) packet.ConnectCode
	queueCapacity int32
}

func NewWebSocket(logger *xlog.Logger, queueCapacity int32) Transport {
	return &transportWebSocket{
		logger:        logger,
		queueCapacity: queueCapacity,
	}
}
func (l *transportWebSocket) Close(ctx context.Context) error {
	return l.httpServer.Close()
}
func (l *transportWebSocket) OnClose(handler func(c Conn)) {
	l.closeHandler = handler
}
func (l *transportWebSocket) OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *transportWebSocket) OnPacket(handler func(p packet.Packet, c Conn)) {
	l.packetHandler = handler
}
func (l *transportWebSocket) Listen(address string) error {
	l.httpServer = &http.Server{
		Addr: address,
		Handler: websocket.Server{
			Handler: func(conn *websocket.Conn) {
				l.handleConn(conn)
			},
			Handshake: func(c *websocket.Config, r *http.Request) (err error) {
				c.Origin, err = websocket.Origin(c, r)
				if err == nil && c.Origin == nil {
					return fmt.Errorf("null origin")
				}
				if c.Protocol[0] != "cable" {
					return fmt.Errorf("invalid protocol")
				}
				return nil
			},
		},
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
	c := newWSConn(connPacket.Identity, conn.RemoteAddr().String(), conn, l.logger, l.queueCapacity)
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
		p, err := packet.ReadFrom(conn)
		if err != nil {
			c.CloseClode(packet.AsCloseCode(err))
			return
		}
		go l.packetHandler(p, c)
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
func newWSConn(id *packet.Identity, ip string, raw *websocket.Conn, logger *xlog.Logger, queueCapacity int32) Conn {
	t := &wsConn{
		id:  id,
		ip:  ip,
		raw: raw,
	}
	t.raw.PayloadType = websocket.BinaryFrame
	return newConn(t, logger, queueCapacity)
}
