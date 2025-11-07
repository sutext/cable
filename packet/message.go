package packet

import (
	"bytes"
	"fmt"
)

type Message struct {
	Flags   uint8
	Channel string
	Payload []byte
}

func NewMessage(payload []byte) *Message {
	return &Message{
		Payload: payload,
	}
}
func (p *Message) String() string {
	return fmt.Sprintf("Message(%d bytes)", len(p.Payload))
}
func (p *Message) Type() PacketType {
	return MESSAGE
}
func (p *Message) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	otherData := other.(*Message)
	return bytes.Equal(p.Payload, otherData.Payload)
}

func (p *Message) EncodeTo(w Encoder) error {
	w.WriteUInt8(p.Flags)
	w.WriteString(p.Channel)
	w.WriteBytes(p.Payload)
	return nil
}
func (p *Message) DecodeFrom(r Decoder) error {
	flags, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Flags = flags
	channel, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Channel = channel
	payload, err := r.ReadAll()
	if err != nil {
		return err
	}
	p.Payload = payload
	return nil
}
