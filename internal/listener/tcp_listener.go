package listener

import (
	"context"
	"log/slog"
	"net"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type tcpListener struct {
	logger        *xlog.Logger
	listener      *net.TCPListener
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(*packet.Connect, *Conn) packet.ConnectCode
}

func NewTCP() Listener {
	return &tcpListener{
		logger: xlog.With("GROUP", "SERVER"),
	}
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
		conn.Close()
	})
	p, err := packet.ReadFrom(conn)
	if err != nil {
		l.logger.Error("failed to read packet", err)
		conn.Close()
		return
	}
	if p.Type() != packet.CONNECT {
		l.logger.Errorf("first packet is not connect packet", slog.String("packetType", p.Type().String()))
		conn.Close()
		return
	}
	connPacket := p.(*packet.Connect)
	c := newTCPConn(connPacket.Identity, conn)
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ClosePacket(packet.NewConnack(code))
		return
	}
	timer.Stop()
	c.SendPacket(packet.NewConnack(packet.ConnectAccepted))
	for {
		p, err := packet.ReadFrom(conn)
		if err != nil {
			c.ClosePacket(packet.NewClose(packet.AsCloseCode(err)))
			return
		}
		c.recvQueue.AddTask(func() {
			l.packetHandler(p, c)
		})
	}
}
