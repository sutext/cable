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

// StatusCode defines the result status of a request.
type StatusCode uint8

// StatusCode constants.
const (
	StatusOK            StatusCode = 0   // Request completed successfully
	StatusNotFound      StatusCode = 100 // Resource not found
	StatusUnauthorized  StatusCode = 101 // Unauthorized access
	StatusInternalError StatusCode = 102 // Internal server error
	StatusInvalidParams StatusCode = 103 // Invalid request parameters
	StatusForbidden     StatusCode = 201 // Access forbidden
	StatusBadRequest    StatusCode = 255 // Invalid request format
)

// Response represents a response packet to a request.
type Response struct {
	packet            // Inherits property management
	id     uint16     // Request identifier being responded to
	Code   StatusCode // Response status code
	Body   []byte     // Response body data
}

// ID returns the request identifier being responded to.
func (p *Response) ID() uint16 {
	return p.id
}

// Type returns the packet type (RESPONSE).
func (p *Response) Type() PacketType {
	return RESPONSE
}

// Equal compares two RESPONSE packets for equality.
func (p *Response) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != RESPONSE {
		return false
	}
	o := other.(*Response)
	return maps.Equal(p.props, o.props) &&
		p.id == o.id &&
		p.Code == o.Code &&
		bytes.Equal(p.Body, o.Body)
}

// String returns a string representation of the RESPONSE packet.
func (p *Response) String() string {
	return fmt.Sprintf("RESPONSE(ID=%d, Code=%d, Props=%v, Content=%d)", p.id, p.Code, p.props, len(p.Body))
}

// WriteTo encodes the RESPONSE packet to the provided encoder.
func (p *Response) WriteTo(w coder.Encoder) error {
	w.WriteUInt16(p.id)
	w.WriteUInt8(uint8(p.Code))
	if err := p.packet.WriteTo(w); err != nil {
		return err
	}
	w.WriteBytes(p.Body)
	return nil
}

// ReadFrom decodes the RESPONSE packet from the provided decoder.
func (p *Response) ReadFrom(r coder.Decoder) error {
	id, err := r.ReadUInt16()
	if err != nil {
		return err
	}
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	if err = p.packet.ReadFrom(r); err != nil {
		return err
	}
	b, err := r.ReadAll()
	if err != nil {
		return err
	}
	p.id = id
	p.Code = StatusCode(code)
	p.Body = b
	return nil
}
