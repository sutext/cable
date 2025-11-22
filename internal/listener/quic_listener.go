package listener

// import (
// 	"context"
// 	"net"
// 	"time"

// 	qc "golang.org/x/net/quic"

// 	"sutext.github.io/cable/packet"
// )

// type quicListener struct {
// 	config        *qc.Config
// 	packetHandler func(p packet.Packet, id *packet.Identity)
// 	acceptHandler func(*packet.Connect, Conn) packet.ConnectCode
// }

// func NewQUIC() Listener {
// 	return &quicListener{}
// }
// func (l *quicListener) OnAccept(handler func(*packet.Connect, Conn) packet.ConnectCode) {
// 	l.acceptHandler = handler
// }
// func (l *quicListener) OnPacket(handler func(p packet.Packet, id *packet.Identity)) {
// 	l.packetHandler = handler
// }
// func (l *quicListener) Listen(address string) error {
// 	listener, err := qc.Listen("udp", address, l.config)
// 	if err != nil {
// 		return err
// 	}
// 	defer listener.Close(context.Background())
// 	for {
// 		conn, err := listener.Accept(context.Background())
// 		if err != nil {
// 			return err
// 		}
// 		go l.handleConn(conn)
// 	}
// }

// func (l *quicListener) handleConn(conn *qc.Conn) {
// 	s, err := conn.AcceptStream(context.Background())
// 	if err != nil {
// 		return
// 	}
// 	c.stream = s
// 	for {
// 		p, err := packet.ReadFrom(c.stream)
// 		if err != nil {
// 			switch err.(type) {
// 			case packet.Error:
// 				c.Close(packet.CloseInvalidPacket)
// 			default:
// 				c.Close(packet.CloseInternalError)
// 			}
// 			return
// 		}
// 		go c.handlePacket(p)
// 	}

// 	defer conn.Close()
// 	ch := make(chan connPacketCloseCode)
// 	defer close(ch)
// 	go l.readConnect(conn, ch)
// 	select {
// 	case r := <-ch:
// 		if r.connPacket == nil {
// 			packet.WriteTo(conn, packet.NewClose(r.closeCode))
// 			return
// 		}
// 		c := newTCPConn(r.connPacket.Identity, conn)
// 		code := l.acceptHandler(r.connPacket, c)
// 		if code != packet.ConnectAccepted {
// 			c.WritePacket(packet.NewConnack(code))
// 			return
// 		}
// 		c.WritePacket(packet.NewConnack(packet.ConnectAccepted))
// 		for {
// 			p, err := packet.ReadFrom(conn)
// 			if err != nil {
// 				code := closeCodeOf(err)
// 				c.WritePacket(packet.NewClose(code))
// 				return
// 			}
// 			l.packetHandler(p, c.id)
// 		}
// 	case <-time.After(time.Second * 10):
// 		packet.WriteTo(conn, packet.NewClose(packet.CloseAuthenticationTimeout))
// 		return
// 	}
// }

// func (l *quicListener) readConnect(conn *net.TCPConn, ch chan<- connPacketCloseCode) {
// 	p, err := packet.ReadFrom(conn)
// 	if err != nil {
// 		ch <- connPacketCloseCode{
// 			closeCode: closeCodeOf(err),
// 		}
// 		return
// 	}
// 	if p.Type() != packet.CONNECT {
// 		ch <- connPacketCloseCode{
// 			closeCode: packet.CloseInvalidPacket,
// 		}
// 		return
// 	}
// 	ch <- connPacketCloseCode{
// 		connPacket: p.(*packet.Connect),
// 	}
// }

// // Below is the tcpConn implementation of Conn interface
// type quicConn struct {
// 	id  *packet.Identity
// 	raw *qc.Conn
// }

// func newQUICConn(id *packet.Identity, raw *qc.Conn) *quicConn {
// 	return &quicConn{
// 		id:  id,
// 		raw: raw,
// 	}
// }
// func (c *quicConn) ID() *packet.Identity {
// 	return c.id
// }
// func (c *quicConn) Close() error {
// 	return c.raw.Close()
// }
// func (c *quicConn) WritePacket(p packet.Packet) error {
// 	return packet.WriteTo(c.raw, p)
// }
