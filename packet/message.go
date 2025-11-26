package packet

import (
	"bytes"
	"fmt"

	"sutext.github.io/cable/coder"
)

type MessageQos int8

const (
	MessageQos0 MessageQos = 0
	MessageQos1 MessageQos = 1
)

const (
	qosMask = 1
	dupMask = 1 << 1
)

// Message represents a message packet.
type Message struct {
	ID      int64
	Qos     MessageQos
	Dup     bool
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

func (p *Message) WriteTo(w coder.Encoder) error {
	flags := uint8(p.Qos)
	if p.Dup {
		flags |= dupMask
	}
	w.WriteUInt8(flags)
	w.WriteInt64(p.ID)
	w.WriteString(p.Channel)
	w.WriteBytes(p.Payload)
	return nil
}
func (p *Message) ReadFrom(r coder.Decoder) error {
	flags, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	id, err := r.ReadInt64()
	if err != nil {
		return err
	}
	channel, err := r.ReadString()
	if err != nil {
		return err
	}

	payload, err := r.ReadAll()
	if err != nil {
		return err
	}
	p.ID = id
	p.Dup = flags&dupMask != 0
	p.Qos = MessageQos(flags & qosMask)
	p.Channel = channel
	p.Payload = payload
	return nil
}

type Messack struct {
	ID int64
}

func NewMessack(id int64) *Messack {
	return &Messack{
		ID: id,
	}
}
func (p *Messack) String() string {
	return fmt.Sprintf("Messack(%d)", p.ID)
}
func (p *Messack) Type() PacketType {
	return MESSACK
}
func (p *Messack) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	otherData := other.(*Messack)
	return p.ID == otherData.ID
}
func (p *Messack) WriteTo(w coder.Encoder) error {
	w.WriteInt64(p.ID)
	return nil
}
func (p *Messack) ReadFrom(r coder.Decoder) error {
	id, err := r.ReadInt64()
	if err != nil {
		return err
	}
	p.ID = id
	return nil
}
