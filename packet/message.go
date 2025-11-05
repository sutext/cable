package packet

import (
	"bytes"
	"fmt"

	"sutext.github.io/cable/internal/buffer"
)

type MessagePacket struct {
	Flags   uint8
	Channel string
	Payload []byte
}

func NewMessage(payload []byte) *MessagePacket {
	return &MessagePacket{
		Payload: payload,
	}
}
func (p *MessagePacket) String() string {
	return fmt.Sprintf("Message(%d bytes)", len(p.Payload))
}
func (p *MessagePacket) Type() PacketType {
	return MESSAGE
}
func (p *MessagePacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	otherData := other.(*MessagePacket)
	return bytes.Equal(p.Payload, otherData.Payload)
}

func (p *MessagePacket) WriteTo(buf *buffer.Buffer) error {
	buf.WriteUInt8(p.Flags)
	buf.WriteString(p.Channel)
	buf.WriteBytes(p.Payload)
	return nil
}

func (p *MessagePacket) ReadFrom(buf *buffer.Buffer) error {
	flags, err := buf.ReadUInt8()
	if err != nil {
		return err
	}
	p.Flags = flags
	channel, err := buf.ReadString()
	if err != nil {
		return err
	}
	p.Channel = channel
	payload, err := buf.ReadAll()
	if err != nil {
		return err
	}
	p.Payload = payload
	return nil
}
