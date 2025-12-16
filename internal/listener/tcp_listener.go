package listener

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/metrics"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type tcpListener struct {
	logger        *xlog.Logger
	listener      *net.TCPListener
	closeHandler  func(c *Conn)
	packetHandler func(p packet.Packet, c *Conn)
	acceptHandler func(*packet.Connect, *Conn) packet.ConnectCode
	queueCapacity int32
}

func NewTCP(queueCapacity, pollCapacity int32, logger *xlog.Logger) Listener {
	return &tcpListener{
		logger:        logger,
		queueCapacity: queueCapacity,
	}
}
func (l *tcpListener) OnClose(handler func(c *Conn)) {
	l.closeHandler = handler
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
	timer.Stop()
	if err != nil {
		l.logger.Error("failed to read packet", xlog.Err(err))
		conn.Close()
		return
	}
	if p.Type() != packet.CONNECT {
		l.logger.Error("first packet is not connect packet", xlog.Str("packetType", p.Type().String()))
		conn.Close()
		return
	}
	connPacket := p.(*packet.Connect)
	tc := newTCPConn(connPacket.Identity, conn, l.queueCapacity)
	c := newConn(tc)
	tc.closeHandler = func() {
		l.closeHandler(c)
	}
	code := l.acceptHandler(connPacket, c)
	if code != packet.ConnectAccepted {
		c.ClosePacket(packet.NewConnack(code))
		return
	}
	c.SendPacket(packet.NewConnack(packet.ConnectAccepted))
	for {
		p, err := packet.ReadFrom(conn)
		if err != nil {
			c.ClosePacket(packet.NewClose(packet.AsCloseCode(err)))
			return
		}
		go l.packetHandler(p, c)
	}
}

type tcpConn struct {
	id           *packet.Identity
	raw          *net.TCPConn
	closed       atomic.Bool
	sendMeter    metrics.Meter
	writeMeter   metrics.Meter
	sendQueue    *queue.Queue
	wirteTimeout time.Duration
	closeHandler func()
}

func (c *tcpConn) ID() *packet.Identity {
	return c.id
}
func (c *tcpConn) close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.sendQueue.Close()
		err := c.raw.Close()
		if c.closeHandler != nil {
			c.closeHandler()
		}
		return err
	}
	return nil
}
func (c *tcpConn) isClosed() bool {
	return c.closed.Load()
}
func (c *tcpConn) writePacket(p packet.Packet, jump bool) error {
	c.sendMeter.Mark(1)
	data, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	if jump {
		return c.sendQueue.Jump(func() {
			c.raw.SetWriteDeadline(time.Now().Add(c.wirteTimeout))
			_, err := c.raw.Write(data)
			c.writeMeter.Mark(1)
			if err != nil {
				c.close()
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.wirteTimeout)
	defer cancel()
	return c.sendQueue.Push(ctx, func() {
		c.raw.SetWriteDeadline(time.Now().Add(c.wirteTimeout))
		_, err := c.raw.Write(data)
		c.writeMeter.Mark(1)
		if err != nil {
			c.close()
		}
	})
}
func (c *tcpConn) sendQueueLength() int32 {
	return c.sendQueue.Len()
}
func newTCPConn(id *packet.Identity, raw *net.TCPConn, sendQueueCapacity int32) *tcpConn {
	return &tcpConn{
		id:           id,
		raw:          raw,
		wirteTimeout: time.Second * 3,
		writeMeter:   metrics.GetOrRegisterMeter("tcp.write", metrics.DefaultRegistry),
		sendMeter:    metrics.GetOrRegisterMeter("tcp.send", metrics.DefaultRegistry),
		sendQueue:    queue.New(sendQueueCapacity),
	}
}
