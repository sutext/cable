package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type MessageQos uint8

const (
	MessageQos0 MessageQos = 0
	MessageQos1 MessageQos = 1
)

type MessageKind uint8

const (
	MessageKindNone MessageKind = 0
)

const (
	qosMask  = 0x80
	dupMask  = 0x40
	kindMask = 0x3f
)

// Message represents a message packet.
type Message struct {
	packet
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
func (p *Message) Type() PacketType {
	return MESSAGE
}
func (p *Message) String() string {
	return fmt.Sprintf("MESSAGE(ID=%d, Qos=%d, Dup=%t, Kind=%d, Props=%v, Payload=%d)", p.ID, p.Qos, p.Dup, p.Kind, p.props, len(p.Payload))
}
func (p *Message) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Message)
	return p.ID == o.ID &&
		p.Qos == o.Qos &&
		p.Dup == o.Dup &&
		p.Kind == o.Kind &&
		maps.Equal(p.props, o.props) &&
		bytes.Equal(p.Payload, o.Payload)
}

func (p *Message) WriteTo(w coder.Encoder) error {
	var flags uint8
	if p.Qos > 0 {
		flags |= qosMask
	}
	if p.Dup {
		flags |= dupMask
	}
	if p.Kind > kindMask {
		return ErrMessageKindTooLarge
	}
	flags |= uint8(p.Kind)
	w.WriteUInt8(flags)
	w.WriteInt64(p.ID)
	err := p.packet.WriteTo(w)
	if err != nil {
		return err
	}
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
	err = p.packet.ReadFrom(r)
	if err != nil {
		return err
	}
	payload, err := r.ReadAll()
	if err != nil {
		return err
	}
	p.ID = id
	p.Dup = flags&dupMask != 0
	p.Qos = MessageQos((flags & qosMask) >> 7)
	p.Kind = MessageKind(flags & kindMask)
	p.Payload = payload
	return nil
}

type Messack struct {
	packet
	ID int64
}

func NewMessack(id int64) *Messack {
	return &Messack{
		ID: id,
	}
}
func (p *Messack) Type() PacketType {
	return MESSACK
}
func (p *Messack) String() string {
	return fmt.Sprintf("MESSACK(id=%d)", p.ID)
}
func (p *Messack) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Messack)
	return maps.Equal(p.props, o.props) && p.ID == o.ID
}
func (p *Messack) WriteTo(w coder.Encoder) error {
	w.WriteInt64(p.ID)
	return p.packet.WriteTo(w)
}
func (p *Messack) ReadFrom(r coder.Decoder) (err error) {
	p.ID, err = r.ReadInt64()
	if err != nil {
		return err
	}
	return p.packet.ReadFrom(r)
}
