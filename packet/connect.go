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

// Identity represents user/client identity information used in CONNECT packets.
type Identity struct {
	UserID   string // User ID
	ClientID string // Client ID
	Password string // Password for authentication
}

// WriteTo encodes the Identity to the provided encoder.
func (i *Identity) WriteTo(w coder.Encoder) error {
	w.WriteString(i.UserID)
	w.WriteString(i.ClientID)
	w.WriteString(i.Password)
	return nil
}

// ReadFrom decodes the Identity from the provided decoder.
func (i *Identity) ReadFrom(r coder.Decoder) error {
	userID, err := r.ReadString()
	if err != nil {
		return err
	}
	clientID, err := r.ReadString()
	if err != nil {
		return err
	}
	password, err := r.ReadString()
	if err != nil {
		return err
	}
	i.UserID = userID
	i.ClientID = clientID
	i.Password = password
	return nil
}

// Equal compares two Identity objects for equality.
func (i *Identity) Equal(other *Identity) bool {
	if other == nil {
		return false
	}
	return i.UserID == other.UserID && i.ClientID == other.ClientID && i.Password == other.Password
}

// String returns a string representation of the Identity.
func (i *Identity) String() string {
	return fmt.Sprintf("uid=%s, cid=%s", i.UserID, i.ClientID)
}

// Connect represents a CONNECT packet used to establish a connection.
type Connect struct {
	packet             // Inherits property management
	Version  uint8     // Protocol version
	Identity *Identity // User/client identity information
}

// NewConnect creates a new CONNECT packet with the given identity.
func NewConnect(identity *Identity) *Connect {
	return &Connect{
		Identity: identity,
		Version:  1,
	}
}

// Type returns the packet type (CONNECT).
func (p *Connect) Type() PacketType {
	return CONNECT
}

// String returns a string representation of the CONNECT packet.
func (p *Connect) String() string {
	return fmt.Sprintf("CONNECT(Version=%d, %s, Props=%v)", p.Version, p.Identity.String(), p.props)
}

// Equal compares two CONNECT packets for equality.
func (p *Connect) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNECT {
		return false
	}
	o := other.(*Connect)
	return maps.Equal(p.props, o.props) && p.Version == o.Version && p.Identity.Equal(o.Identity)
}

// WriteTo encodes the CONNECT packet to the provided encoder.
func (p *Connect) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(p.Version)
	err := p.Identity.WriteTo(w)
	if err != nil {
		return err
	}
	return p.packet.WriteTo(w)
}

// ReadFrom decodes the CONNECT packet from the provided decoder.
func (p *Connect) ReadFrom(r coder.Decoder) error {
	version, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	identity := &Identity{}
	err = identity.ReadFrom(r)
	if err != nil {
		return err
	}
	p.Version = version
	p.Identity = identity
	return p.packet.ReadFrom(r)
}

// ConnectCode defines the result of a connection attempt.
type ConnectCode uint8

// ConnectCode constants.
const (
	ConnectAccepted    ConnectCode = 0 // Connection accepted
	ConnectAuthfail    ConnectCode = 1 // Connection rejected
	ConnectDuplicate   ConnectCode = 2 // Duplicate connection detected
	ConnectServerError ConnectCode = 3 // Server error occurred
)

// String returns a string representation of the ConnectCode.
func (c ConnectCode) String() string {
	switch c {
	case ConnectAccepted:
		return "Connection Accepted"
	case ConnectAuthfail:
		return "Connection Rejected"
	case ConnectDuplicate:
		return "Connection Duplicate"
	case ConnectServerError:
		return "Server Error"
	default:
		return "Unknown Connack Code"
	}
}
func (c ConnectCode) Error() string {
	return c.String()
}

// Connack represents a CONNACK packet, the response to a CONNECT packet.
type Connack struct {
	packet             // Inherits property management
	Code   ConnectCode // Connection result code
}

// NewConnack creates a new CONNACK packet with the given result code.
func NewConnack(code ConnectCode) *Connack {
	return &Connack{
		Code: code,
	}
}

// Type returns the packet type (CONNACK).
func (p *Connack) Type() PacketType {
	return CONNACK
}

// Equal compares two CONNACK packets for equality.
func (p *Connack) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNACK {
		return false
	}
	o := other.(*Connack)
	return maps.Equal(p.props, o.props) && p.Code == o.Code
}

// String returns a string representation of the CONNACK packet.
func (p *Connack) String() string {
	return fmt.Sprintf("CONNACK(Code=%s,Props=%v)", p.Code, p.props)
}

// WriteTo encodes the CONNACK packet to the provided encoder.
func (p *Connack) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return p.packet.WriteTo(w)
}

// ReadFrom decodes the CONNACK packet from the provided decoder.
func (p *Connack) ReadFrom(r coder.Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = ConnectCode(code)
	return p.packet.ReadFrom(r)
}
