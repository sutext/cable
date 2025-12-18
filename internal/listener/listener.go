package listener

import (
	"context"

	"sutext.github.io/cable/packet"
)

type Listener interface {
	Listen(addr string) error
	Close(ctx context.Context) error
	OnClose(handler func(c *Conn))
	OnPacket(handler func(p packet.Packet, c *Conn))
	OnAccept(handler func(p *packet.Connect, c *Conn) packet.ConnectCode)
}

type conn interface {
	ID() *packet.Identity
	close() error
	isClosed() bool
	writePacket(ctx context.Context, p packet.Packet, jump bool) error
}
