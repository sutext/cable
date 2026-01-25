package network

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type transportUDP struct {
	logger   *xlog.Logger
	listener *net.UDPConn
	delegate delegate
}

func NewUDP(delegate delegate) Transport {
	return &transportUDP{
		logger:   delegate.Logger(),
		delegate: delegate,
	}
}
func (l *transportUDP) Close(ctx context.Context) error {
	return l.listener.Close()
}

func (l *transportUDP) Listen(address string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	l.listener = listener
	defer listener.Close()
	buf := make([]byte, packet.MAX_UDP)
	for {
		n, addr, err := listener.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		go l.handleConn(listener, addr, p)
	}
}

func (l *transportUDP) handleConn(conn *net.UDPConn, addr *net.UDPAddr, p packet.Packet) {
	if p.Type() != packet.CONNECT {
		cid, ok := p.Get(packet.PropertyClientID)
		if !ok {
			l.logger.Warn("unknown udp packet", xlog.Str("packet", p.String()))
			return
		}
		c, loaded := l.delegate.GetConn(cid)
		if !loaded {
			l.logger.Warn("unknown udp clientID", xlog.Str("clientID", cid))
			return
		}
		go l.delegate.OnPacket(c, p)
		return
	}
	connPacket := p.(*packet.Connect)
	uc := newUDPConn(connPacket.Identity, conn, addr)
	c := newConn(uc, l.delegate)
	if err := l.delegate.OnConnect(c, connPacket); err != nil {
		l.logger.Error("connect error", xlog.Err(err))
		return
	}
	uc.ping = newPinger(c, time.Second*25, time.Second*5)
	uc.ping.Start()
}

type udpConn struct {
	id   *packet.Identity
	raw  *net.UDPConn
	addr atomic.Pointer[net.UDPAddr]
	ping *pinger
}

func (c *udpConn) ID() *packet.Identity {
	return c.id
}
func (c *udpConn) IP() string {
	return c.addr.Load().IP.String()
}
func (c *udpConn) Close() error {
	return nil
}

func (c *udpConn) WriteData(data []byte) error {
	_, err := c.raw.WriteToUDP(data, c.addr.Load())
	return err
}
func newUDPConn(id *packet.Identity, raw *net.UDPConn, addr *net.UDPAddr) *udpConn {
	u := &udpConn{
		id:  id,
		raw: raw,
	}
	u.addr.Store(addr)
	return u
}
