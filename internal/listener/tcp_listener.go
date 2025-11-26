package listener

import (
	"context"
	"net"
	"time"

	"sutext.github.io/cable/internal/result"
	"sutext.github.io/cable/packet"
)

type tcpListener struct {
	listener      *net.TCPListener
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(*packet.Connect, *Conn) packet.ConnectCode
}

func NewTCP() Listener {
	return &tcpListener{}
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
	ch := make(chan result.Result[*packet.Connect])
	defer close(ch)
	go l.readConnect(conn, ch)
	select {
	case r := <-ch:
		if r.Error() != nil {
			packet.WriteTo(conn, packet.NewClose(packet.AsCloseCode(r.Error())))
			conn.Close()
			return
		}
		c := newTCPConn(r.Value().Identity, conn)
		code := l.acceptHandler(r.Value(), c)
		if code != packet.ConnectAccepted {
			c.SendPacket(packet.NewConnack(code))
			c.Close()
			return
		}
		c.SendPacket(packet.NewConnack(packet.ConnectAccepted))
		for {
			p, err := packet.ReadFrom(conn)
			if err != nil {
				c.CloseCode(packet.AsCloseCode(err))
				return
			}
			c.recvQueue.AddTask(func() {
				l.packetHandler(p, c)
			})
		}
	case <-time.After(time.Second * 10):
		packet.WriteTo(conn, packet.NewClose(packet.CloseAuthenticationTimeout))
		conn.Close()
		return
	}
}

func (l *tcpListener) readConnect(conn *net.TCPConn, ch chan<- result.Result[*packet.Connect]) {
	p, err := packet.ReadFrom(conn)
	if err != nil {
		ch <- result.Err[*packet.Connect](err)
		return
	}
	if p.Type() != packet.CONNECT {
		ch <- result.Err[*packet.Connect](packet.CloseInvalidPacket)
		return
	}
	ch <- result.OK(p.(*packet.Connect))
}
