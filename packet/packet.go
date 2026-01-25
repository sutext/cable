// Package packet defines a binary protocol for network communication.
// It provides a comprehensive set of packet types for establishing connections,
// sending messages, making requests, and managing connection lifecycle.
//
// The protocol uses a compact binary format with variable-length encoding
// for efficient network transmission.
package packet

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"sutext.github.io/cable/coder"
)

// Packet size constants.
const (
	MIN_LEN int = 0             // Minimum packet length
	MID_LEN int = 0x3ff         // Medium packet length threshold (10 bits)
	MAX_LEN int = 0x3_ffff_ffff // Maximum packet length (28 bits)
	MAX_UDP int = MID_LEN + 2   // Maximum UDP packet size including header
)

// Error defines error codes for packet processing.
type Error uint8

// Error constants.
const (
	ErrInvalidReadLen      Error = 1 // Invalid length when reading packet
	ErrUnknownPacketType   Error = 2 // Unknown packet type encountered
	ErrPacketSizeTooLarge  Error = 3 // Packet exceeds maximum allowed size
	ErrMessageKindTooLarge Error = 4 // Message kind value is too large
)

// Error returns the string representation of the error.
func (e Error) Error() string {
	switch e {
	case ErrInvalidReadLen:
		return "invalid length when read"
	case ErrUnknownPacketType:
		return "unknown packet type"
	case ErrPacketSizeTooLarge:
		return "packet size too large"
	case ErrMessageKindTooLarge:
		return "message kind too large"
	default:
		return "unknown error"
	}
}

// PacketType defines the type of network packet.
type PacketType uint8

// Packet type constants.
const (
	CONNECT  PacketType = 0  // Client connection request
	CONNACK  PacketType = 1  // Connection acknowledgment
	MESSAGE  PacketType = 2  // Data message
	MESSACK  PacketType = 3  // Message acknowledgment
	REQUEST  PacketType = 4  // Request command
	RESPONSE PacketType = 5  // Request response
	PING     PacketType = 6  // Heartbeat request
	PONG     PacketType = 7  // Heartbeat response
	CLOSE    PacketType = 15 // Connection close notification
)

func (t PacketType) String() string {
	switch t {
	case CONNECT:
		return "CONNECT"
	case CONNACK:
		return "CONNACK"
	case MESSAGE:
		return "MESSAGE"
	case MESSACK:
		return "MESSACK"
	case REQUEST:
		return "REQUEST"
	case RESPONSE:
		return "RESPONSE"
	case PING:
		return "PING"
	case PONG:
		return "PONG"
	case CLOSE:
		return "CLOSE"
	default:
		return "UNKNOWN"
	}
}

// Property defines keys for packet properties.
// These keys are used to store packet properties in a map.
// User can define their own properties by using values greater than 3. 0...3 are reserved for standard properties.
// The property keys are defined as uint8 to save memory.the maximum number of properties is 256.
type Property uint8

// Property constants.
const (
	PropertyNone     Property = 0 // Empty property
	PropertyUserID   Property = 1 // User ID property
	PropertyChannel  Property = 2 // Channel property
	PropertyClientID Property = 3 // Client ID property
)

// Properties defines an interface for managing packet properties.
type Properties interface {
	// Get retrieves a property value by key. Returns the value and a boolean indicating if the key exists.
	Get(key Property) (string, bool)
	// Set sets a property value by key.
	Set(key Property, value string)
}

// Packet defines the interface for all network packets.
// All packets must implement this interface to be sent over the network.
type Packet interface {
	fmt.Stringer       // Provides string representation for debugging
	coder.Codable      // Supports binary encoding/decoding
	Properties         // Supports property management
	Type() PacketType  // Returns the packet type
	Equal(Packet) bool // Compares packets for equality
}

// packet is the base struct that implements the Properties interface.
// All other packet types embed this struct to inherit property management functionality.
type packet struct {
	props map[uint8]string // Map of property keys to values
}

// WriteTo encodes the packet properties to the encoder.
func (p *packet) WriteTo(c coder.Encoder) error {
	c.WriteUInt8Map(p.props)
	return nil
}

// ReadFrom decodes the packet properties from the decoder.
func (p *packet) ReadFrom(c coder.Decoder) error {
	m, err := c.ReadUInt8Map()
	if err != nil {
		return err
	}
	p.props = m
	return nil
}

// Get retrieves a property value by key. Returns the value and a boolean indicating if the key exists.
func (p *packet) Get(key Property) (string, bool) {
	v, ok := p.props[uint8(key)]
	return v, ok
}

// Set sets a property value by key.
func (p *packet) Set(key Property, value string) {
	if p.props == nil {
		p.props = make(map[uint8]string)
	}
	p.props[uint8(key)] = value
}

// ping represents a PING packet used for heartbeat detection.
type ping struct {
	packet // Inherits property management
}

// NewPing creates a new PING packet.
func NewPing() Packet {
	return &ping{}
}

// Type returns the packet type (PING).
func (p *ping) Type() PacketType {
	return PING
}

// Equal compares two PING packets for equality.
func (p *ping) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	return other.Type() == PING && maps.Equal(p.props, other.(*ping).props)
}

// String returns a string representation of the PING packet.
func (p *ping) String() string {
	return fmt.Sprintf("PING(%v)", p.props)
}

// pong represents a PONG packet used for heartbeat response.
type pong struct {
	packet // Inherits property management
}

// NewPong creates a new PONG packet.
func NewPong() Packet {
	return &pong{}
}

// Type returns the packet type (PONG).
func (p *pong) Type() PacketType {
	return PONG
}

// Equal compares two PONG packets for equality.
func (p *pong) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	return other.Type() == PONG && maps.Equal(p.props, other.(*pong).props)
}

// String returns a string representation of the PONG packet.
func (p *pong) String() string {
	return fmt.Sprintf("PONG(Props=%v)", p.props)
}

// Marshal encodes the given Packet object to bytes.
// Uses compact binary encoding for integers and varints.
// Returns the encoded bytes and any error encountered.
func Marshal(p Packet) ([]byte, error) {
	header, data, err := pack(p)
	if err != nil {
		return nil, err
	}
	bytes := make([]byte, len(header)+len(data))
	copy(bytes, header)
	copy(bytes[len(header):], data)
	return bytes, nil
}

// Unmarshal decodes the given bytes into a Packet object.
// Returns the decoded packet and any error encountered.
func Unmarshal(bytes []byte) (Packet, error) {
	return ReadFrom(coder.NewDecoder(bytes))
}

// WriteTo writes the given Packet to the provided io.Writer.
// Encodes the packet to bytes and writes it to the writer.
// Returns any error encountered during writing.
func WriteTo(w io.Writer, p Packet) error {
	header, data, err := pack(p)
	if err != nil {
		return err
	}
	bytes := make([]byte, len(header)+len(data))
	copy(bytes, header)
	copy(bytes[len(header):], data)
	_, err = w.Write(bytes)
	return err
}

// ReadFrom reads a Packet from the provided io.Reader.
// Reads the packet header, determines the packet type and length, then reads the data.
// Returns the decoded packet and any error encountered during reading.
func ReadFrom(r io.Reader) (Packet, error) {
	// read header
	header := make([]byte, 2)
	if _, err := r.Read(header); err != nil {
		return nil, err
	}
	//read length
	byteCount := (header[0] >> 2) & 0x03
	length := uint64(header[0]&0x03)<<8 | uint64(header[1])
	if byteCount > 0 {
		bs := make([]byte, byteCount)
		if _, err := r.Read(bs); err != nil {
			return nil, err
		}
		for _, b := range bs {
			length = length<<8 | uint64(b)
		}
	}
	// read data
	var data []byte
	if length > 0 {
		data = make([]byte, length)
		if _, err := r.Read(data); err != nil {
			return nil, err
		}
	}
	return unpack(header, data)
}

// pack is an internal function that encodes the given Packet object to header and data bytes.
// The header contains the packet type and length, while the data contains the packet payload.
// Returns the header bytes, data bytes, and any error encountered.
func pack(p Packet) (header, data []byte, err error) {
	data, err = coder.Marshal(p)
	if err != nil {
		return nil, nil, err
	}
	length := len(data)
	if length > MAX_LEN {
		return nil, nil, ErrPacketSizeTooLarge
	}
	if length > MID_LEN {
		bs := make([]byte, 0, 5)
		for length > 0 {
			bs = append(bs, byte(length&0xff))
			length >>= 8
		}
		slices.Reverse(bs)
		if bs[0] > 3 {
			header = make([]byte, len(bs)+1)
			copy(header[1:], bs)
		} else {
			header = bs
		}
		header[0] = byte(p.Type()<<4) | byte(len(header)-2)<<2 | header[0]
	} else {
		header = make([]byte, 2)
		header[0] = byte(p.Type()<<4) | byte(length>>8)
		header[1] = byte(length)
	}
	return header, data, nil
}

// unpack is an internal function that decodes the given header and data bytes into the corresponding Packet object.
// Determines the packet type from the header and creates the appropriate packet struct.
// Returns the decoded packet and any error encountered.
func unpack(header, data []byte) (Packet, error) {
	packetType := PacketType(header[0] >> 4)
	switch packetType {
	case CONNECT:
		conn := &Connect{}
		if err := coder.Unmarshal(data, conn); err != nil {
			return nil, err
		}
		return conn, nil
	case CONNACK:
		connack := &Connack{}
		if err := coder.Unmarshal(data, connack); err != nil {
			return nil, err
		}
		return connack, nil
	case MESSAGE:
		msg := &Message{}
		if err := coder.Unmarshal(data, msg); err != nil {
			return nil, err
		}
		return msg, nil
	case MESSACK:
		ack := &Messack{}
		if err := coder.Unmarshal(data, ack); err != nil {
			return nil, err
		}
		return ack, nil
	case REQUEST:
		req := &Request{}
		if err := coder.Unmarshal(data, req); err != nil {
			return nil, err
		}
		return req, nil
	case RESPONSE:
		res := &Response{}
		if err := coder.Unmarshal(data, res); err != nil {
			return nil, err
		}
		return res, nil
	case PING:
		res := &ping{}
		if err := coder.Unmarshal(data, res); err != nil {
			return nil, err
		}
		return res, nil
	case PONG:
		res := &pong{}
		if err := coder.Unmarshal(data, res); err != nil {
			return nil, err
		}
		return res, nil
	case CLOSE:
		close := &Close{}
		if err := coder.Unmarshal(data, close); err != nil {
			return nil, err
		}
		return close, nil
	default:
		return nil, ErrUnknownPacketType
	}
}
