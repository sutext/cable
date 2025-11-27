package packet

import (
	"bytes"
	"fmt"

	"sutext.github.io/cable/coder"
)

type MessageQos uint8

const (
	MessageQos0 MessageQos = 0
	MessageQos1 MessageQos = 1
)

type MessageKind uint8

const (
	qosMask  = 0x80
	dupMask  = 0x40
	kindMask = 0x3f
)

// Message represents a message packet.
type Message struct {
	ID      int64
	Qos     MessageQos
	Dup     bool
	Kind    MessageKind
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
	if p.Kind > kindMask {
		return ErrMessageKindTooLarge
	}
	flags |= uint8(p.Kind)
	w.WriteUInt8(flags)
	w.WriteInt64(p.ID)
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
	payload, err := r.ReadAll()
	if err != nil {
		return err
	}
	p.ID = id
	p.Dup = flags&dupMask != 0
	p.Qos = MessageQos(flags & qosMask)
	p.Kind = MessageKind(flags & kindMask)
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
