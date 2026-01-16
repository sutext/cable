// Package packet defines a binary protocol for network communication.
// It provides a comprehensive set of packet types for establishing connections,
// sending messages, making requests, and managing connection lifecycle.
//
// The protocol uses a compact binary format with variable-length encoding
// for efficient network transmission.
package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

// MessageQos defines the quality of service level for messages.
type MessageQos uint8

// MessageQos constants.
const (
	MessageQos0 MessageQos = 0 // At most once delivery
	MessageQos1 MessageQos = 1 // At least once delivery
)

// MessageKind defines the type of message payload.
// User can define their own message kinds by extending this enum. 0...3 are reserved for system message kinds.
// The maximum value of MessageKind is 63, which is the maximum value of the kindMask constant.
type MessageKind uint8

// MessageKind constants.
const (
	MessageKindNone      MessageKind = 0 // The default zero value. No specific behavior.
	MessageKindToAll     MessageKind = 1 // Broker will auto broadcast message to all clients.
	MessageKindToUser    MessageKind = 2 // Broker will auto resend message to a specific user. The userID exsits in the props map.
	MessageKindToChannel MessageKind = 3 // Broker will auto resend message to a specific channel. The channel exsits in the props map.
)

// Bitmask constants for message flags.
const (
	qosMask  = 0x80 // Quality of Service flag mask
	dupMask  = 0x40 // Duplicate message flag mask
	kindMask = 0x3f // Message kind flag mask
)

// Message represents a data message packet.
type Message struct {
	packet              // Inherits property management
	ID      uint16      // Message identifier
	Qos     MessageQos  // Quality of service level
	Dup     bool        // Duplicate message flag
	Kind    MessageKind // Message kind
	Payload []byte      // Message payload data
}

// NewMessage creates a new MESSAGE packet with the given payload.
func NewMessage(payload []byte) *Message {
	return &Message{
		Payload: payload,
	}
}

// Ack creates a MESSACK packet for this message.
func (p *Message) Ack() *Messack {
	return &Messack{
		id: p.ID,
	}
}

// Type returns the packet type (MESSAGE).
func (p *Message) Type() PacketType {
	return MESSAGE
}

// Equal compares two MESSAGE packets for equality.
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

// String returns a string representation of the MESSAGE packet.
func (p *Message) String() string {
	return fmt.Sprintf("MESSAGE(ID=%d, Qos=%d, Dup=%t, Kind=%d, Props=%v, Payload=%d)", p.ID, p.Qos, p.Dup, p.Kind, p.props, len(p.Payload))
}

// WriteTo encodes the MESSAGE packet to the provided encoder.
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
	w.WriteUInt16(p.ID)
	err := p.packet.WriteTo(w)
	if err != nil {
		return err
	}
	w.WriteBytes(p.Payload)
	return nil
}

// ReadFrom decodes the MESSAGE packet from the provided decoder.
func (p *Message) ReadFrom(r coder.Decoder) error {
	flags, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	id, err := r.ReadUInt16()
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

// Messack represents a message acknowledgment packet.
type Messack struct {
	packet        // Inherits property management
	id     uint16 // Message identifier being acknowledged
}

// ID returns the message identifier being acknowledged.
func (p *Messack) ID() uint16 {
	return p.id
}

// Type returns the packet type (MESSACK).
func (p *Messack) Type() PacketType {
	return MESSACK
}

// Equal compares two MESSACK packets for equality.
func (p *Messack) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Messack)
	return maps.Equal(p.props, o.props) && p.id == o.id
}

// String returns a string representation of the MESSACK packet.
func (p *Messack) String() string {
	return fmt.Sprintf("MESSACK(id=%d)", p.id)
}

// WriteTo encodes the MESSACK packet to the provided encoder.
func (p *Messack) WriteTo(w coder.Encoder) error {
	w.WriteUInt16(p.id)
	return p.packet.WriteTo(w)
}

// ReadFrom decodes the MESSACK packet from the provided decoder.
func (p *Messack) ReadFrom(r coder.Decoder) (err error) {
	p.id, err = r.ReadUInt16()
	if err != nil {
		return err
	}
	return p.packet.ReadFrom(r)
}
