package listener

import (
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
)

type Conn interface {
	ID() *packet.Identity
	Close() error
	WritePacket(p packet.Packet) error
}

type Listener interface {
	Listen(addr string) error
	OnPacket(handler func(p packet.Packet, id *packet.Identity))
	OnAccept(handler func(p *packet.Connect, c Conn) packet.ConnectCode)
}

func closeCodeOf(err error) packet.CloseCode {
	switch err.(type) {
	case packet.Error, coder.Error:
		return packet.CloseInvalidPacket
	default:
		return packet.CloseInternalError
	}
}
