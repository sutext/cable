package listener

import "sutext.github.io/cable/packet"

type Conn interface {
	Dail() error
	Close() error
	WritePacket(p packet.Packet) error
	ReadPacket() (packet.Packet, error)
}
