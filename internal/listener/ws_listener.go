package listener

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type wsListener struct {
	logger        *xlog.Logger
	httpServer    *http.Server
	closeHandler  func(c Conn)
	packetHandler func(p packet.Packet, c Conn)
	acceptHandler func(p *packet.Connect, c Conn) packet.ConnectCode
	queueCapacity int32
}

func NewWS(logger *xlog.Logger, queueCapacity int32) Listener {
	return &wsListener{
		logger:        logger,
		queueCapacity: queueCapacity,
	}
}
func (l *wsListener) Close(ctx context.Context) error {
	return l.httpServer.Close()
}
func (l *wsListener) OnClose(handler func(c Conn)) {
	l.closeHandler = handler
}
func (l *wsListener) OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode) {
	l.acceptHandler = handler
}
func (l *wsListener) OnPacket(handler func(p packet.Packet, c Conn)) {
	l.packetHandler = handler
}

func (l *wsListener) Listen(address string) error {
	ws := websocket.Server{
		Config: websocket.Config{},
		Handler: func(conn *websocket.Conn) {
			l.handleConn(conn)
		},
		Handshake: func(c *websocket.Config, r *http.Request) (err error) {
			c.Origin, err = websocket.Origin(c, r)
			if err == nil && c.Origin == nil {
				return fmt.Errorf("null origin")
			}
			return err
		},
	}
	return http.ListenAndServe(address, ws)
}
func (l *wsListener) handleConn(conn *websocket.Conn) {
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
	c := newWSConn(connPacket.Identity, conn, l.logger, l.queueCapacity)
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
	raw *websocket.Conn
	id  *packet.Identity
}

func (c *wsConn) ID() *packet.Identity {
	return c.id
}
func (c *wsConn) Close() error {
	return c.raw.Close()
}

func (c *wsConn) WriteData(data []byte) error {
	_, err := c.raw.Write(data)
	return err
}
func newWSConn(id *packet.Identity, raw *websocket.Conn, logger *xlog.Logger, queueCapacity int32) Conn {
	t := &wsConn{
		id:  id,
		raw: raw,
	}
	return newConn(t, logger, queueCapacity)
}
