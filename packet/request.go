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

// Request represents a request packet for remote procedure calls.
type Request struct {
	packet        // Inherits property management
	ID     uint16 // Request identifier
	Method string // Request method name
	Body   []byte // Request body data
}

// NewRequest creates a new REQUEST packet with the given method and optional body.
func NewRequest(method string, body ...[]byte) *Request {
	var b []byte
	if len(body) > 0 {
		b = body[0]
	}
	return &Request{
		Method: method,
		Body:   b,
	}
}

// Type returns the packet type (REQUEST).
func (p *Request) Type() PacketType {
	return REQUEST
}

// Equal compares two REQUEST packets for equality.
func (p *Request) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Request)
	return maps.Equal(p.props, o.props) &&
		p.ID == o.ID &&
		p.Method == o.Method &&
		bytes.Equal(p.Body, o.Body)
}

// String returns a string representation of the REQUEST packet.
func (p *Request) String() string {
	return fmt.Sprintf("REQUEST(ID=%d, Method=%s, Props=%v, Content=%d)", p.ID, p.Method, p.props, len(p.Body))
}

// Response creates a RESPONSE packet for this request.
func (p *Request) Response(code StatusCode, content ...[]byte) *Response {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Response{
		id:   p.ID,
		Code: code,
		Body: b,
	}
}

// WriteTo encodes the REQUEST packet to the provided encoder.
func (p *Request) WriteTo(w coder.Encoder) error {
	w.WriteUInt16(p.ID)
	w.WriteString(p.Method)
	err := p.packet.WriteTo(w)
	if err != nil {
		return err
	}
	w.WriteBytes(p.Body)
	return nil
}

// ReadFrom decodes the REQUEST packet from the provided decoder.
func (p *Request) ReadFrom(r coder.Decoder) error {
	var err error
	if p.ID, err = r.ReadUInt16(); err != nil {
		return err
	}
	if p.Method, err = r.ReadString(); err != nil {
		return err
	}
	if err = p.packet.ReadFrom(r); err != nil {
		return err
	}
	if p.Body, err = r.ReadAll(); err != nil {
		return err
	}
	return nil
}
