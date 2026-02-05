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
	"strconv"
	"strings"

	"sutext.github.io/cable/coder"
)

// MessageKind defines the kint of message payload.
// User can define their own message kinds by extending this enum. 0 reserved for system message kinds.
// The maximum value of MessageKind is 63, which is the maximum value of the kindMask constant.
type MessageKind uint8

// MessageKind constants.
const (
	MessageKindNone MessageKind = 0 // The default zero value. No specific behavior.
)

func ParseKind(k string) (kind MessageKind, err error) {
	k = strings.TrimSpace(k)
	i, err := strconv.ParseInt(k, 10, 32)
	if err != nil {
		return kind, fmt.Errorf("invalid message kind: %s", k)
	}
	if i < 0 || i > 0xff {
		return kind, fmt.Errorf("invalid message kind: %s", k)
	}
	return MessageKind(i), nil
}

// Message represents a data message packet.
type Message struct {
	packet              // Inherits property management
	Kind    MessageKind // Message kind
	Payload []byte      // Message payload data
}

// NewMessage creates a new MESSAGE packet with the given payload.
func NewMessage(payload []byte) *Message {
	return &Message{
		Payload: payload,
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
	return p.Kind == o.Kind &&
		maps.Equal(p.props, o.props) &&
		bytes.Equal(p.Payload, o.Payload)
}

// String returns a string representation of the MESSAGE packet.
func (p *Message) String() string {
	return fmt.Sprintf("MESSAGE(Kind=%d, Props=%v, Payload=%d)", p.Kind, p.props, len(p.Payload))
}

// WriteTo encodes the MESSAGE packet to the provided encoder.
func (p *Message) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Kind))
	err := p.packet.WriteTo(w)
	if err != nil {
		return err
	}
	w.WriteBytes(p.Payload)
	return nil
}

// ReadFrom decodes the MESSAGE packet from the provided decoder.
func (p *Message) ReadFrom(r coder.Decoder) error {
	kind, err := r.ReadUInt8()
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
	p.Kind = MessageKind(kind)
	p.Payload = payload
	return nil
}
