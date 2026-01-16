// Package packet defines a binary protocol for network communication.
// It provides a comprehensive set of packet types for establishing connections,
// sending messages, making requests, and managing connection lifecycle.
//
// The protocol uses a compact binary format with variable-length encoding
// for efficient network transmission.
package packet

import (
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

// CloseCode defines the reason for a connection closure.
type CloseCode uint8

// CloseCode constants.
const (
	CloseNormal         CloseCode = iota // Normal connection closure
	CloseKickedOut                       // Client was kicked out by server
	CloseNoHeartbeat                     // No heartbeat received
	ClosePingTimeOut                     // Ping request timed out
	CloseAuthFailure                     // Authentication failed
	CloseAuthTimeout                     // Authentication timed out
	CloseInvalidPacket                   // Invalid packet received
	CloseInternalError                   // Internal server error
	CloseDuplicateLogin                  // Duplicate login attempt
	CloseServerShutdown                  // Server is shutting down
	CloseServerExpeled                   // Server expelled client
)

var closeCodeMap = map[CloseCode]string{
	CloseNormal:         "Normal",
	CloseKickedOut:      "Kicked Out",
	CloseNoHeartbeat:    "No Heartbeat",
	ClosePingTimeOut:    "Ping Time Out",
	CloseAuthFailure:    "Authentication Failure",
	CloseAuthTimeout:    "Authentication Timeout",
	CloseInvalidPacket:  "Invalid Packet",
	CloseInternalError:  "Internal Error",
	CloseDuplicateLogin: "Duplicate Login",
	CloseServerShutdown: "Server Shutdown",
	CloseServerExpeled:  "Server Expeled",
}

// String returns a string representation of the CloseCode.
func (c CloseCode) String() string {
	if s, ok := closeCodeMap[c]; ok {
		return s
	}
	return "Unknown"
}

// Error implements the error interface for CloseCode.
func (c CloseCode) Error() string {
	return c.String()
}

// AsCloseCode converts an error to a CloseCode.
// Maps specific error types to appropriate CloseCode values.
func AsCloseCode(err error) CloseCode {
	switch e := err.(type) {
	case CloseCode:
		return e
	case Error, coder.Error:
		return CloseInvalidPacket
	default:
		return CloseInternalError
	}
}

// Close represents a CLOSE packet used to notify connection closure.
type Close struct {
	packet           // Inherits property management
	Code   CloseCode // Reason for connection closure
}

// NewClose creates a new CLOSE packet with the given close code.
func NewClose(code CloseCode) *Close {
	return &Close{
		Code: code,
	}
}

// Type returns the packet type (CLOSE).
func (p *Close) Type() PacketType {
	return CLOSE
}

// Equal compares two CLOSE packets for equality.
func (p *Close) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CLOSE {
		return false
	}
	o := other.(*Close)
	return maps.Equal(p.props, o.props) && p.Code == o.Code
}

// String returns a string representation of the CLOSE packet.
func (p *Close) String() string {
	return fmt.Sprintf("CLOSE(Code=%s, Props=%v)", p.Code, p.props)
}

// WriteTo encodes the CLOSE packet to the provided encoder.
func (p *Close) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return p.packet.WriteTo(w)
}

// ReadFrom decodes the CLOSE packet from the provided decoder.
func (p *Close) ReadFrom(r coder.Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = CloseCode(code)
	return p.packet.ReadFrom(r)
}
