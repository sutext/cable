package listener

import (
	"context"

	"sutext.github.io/cable/packet"
)

type Conn interface {
	ID() *packet.Identity
	Close() error
	WritePacket(p packet.Packet) error
}

type Listener interface {
	Listen(addr string) error
	Close(ctx context.Context) error
	OnPacket(handler func(p packet.Packet, id *packet.Identity))
	OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode)
}
