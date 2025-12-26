package packet

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"sutext.github.io/cable/coder"
)

const (
	MIN_LEN int = 0
	MID_LEN int = 0x3ff
	MAX_LEN int = 0x3_ffff_ffff
	MAX_UDP int = MID_LEN + 2 // max UDP packet size
)

// Error represents an error code.
type Error uint8

const (
	ErrInvalidReadLen      Error = 1
	ErrUnknownPacketType   Error = 2
	ErrPacketSizeTooLarge  Error = 3
	ErrMessageKindTooLarge Error = 4
)

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

// PacketType represents a packet type.
type PacketType uint8

const (
	CONNECT  PacketType = 0  // CONNECT packet
	CONNACK  PacketType = 1  // CONNACK packet
	MESSAGE  PacketType = 2  // MESSAGE packet
	MESSACK  PacketType = 3  // MASSACK packet
	REQUEST  PacketType = 4  // REQUEST packet
	RESPONSE PacketType = 5  // RESPONSE packet
	PING     PacketType = 6  // PING packet
	PONG     PacketType = 7  // PONG packet
	CLOSE    PacketType = 15 // CLOSE packet
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

type Property uint8

const (
	PropertyConnID  Property = 1
	PropertyUserID  Property = 2
	PropertyChannel Property = 3
)

type Properties interface {
	Get(key Property) (string, bool)
	Set(key Property, value string)
}
type Packet interface {
	fmt.Stringer
	coder.Codable
	Properties
	Type() PacketType
	Equal(Packet) bool
}
type packet struct {
	props map[uint8]string
}

func (p *packet) WriteTo(c coder.Encoder) error {
	c.WriteUInt8Map(p.props)
	return nil
}
func (p *packet) ReadFrom(c coder.Decoder) error {
	m, err := c.ReadUInt8Map()
	if err != nil {
		return err
	}
	p.props = m
	return nil
}
func (p *packet) Get(key Property) (string, bool) {
	v, ok := p.props[uint8(key)]
	return v, ok
}
func (p *packet) Set(key Property, value string) {
	if p.props == nil {
		p.props = make(map[uint8]string)
	}
	p.props[uint8(key)] = value
}

type ping struct {
	packet
}

func NewPing() Packet {
	return &ping{}
}
func (p *ping) Type() PacketType {
	return PING
}
func (p *ping) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	return other.Type() == PING && maps.Equal(p.props, other.(*ping).props)
}
func (p *ping) String() string {
	return fmt.Sprintf("PING(%v)", p.props)
}

type pong struct {
	packet
}

func NewPong() Packet {
	return &pong{}
}
func (p *pong) Type() PacketType {
	return PONG
}
func (p *pong) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	return other.Type() == PONG && maps.Equal(p.props, other.(*pong).props)
}
func (p *pong) String() string {
	return fmt.Sprintf("PONG(Props=%v)", p.props)
}

// Marshal encodes the given Packet object to bytes.
// Use compact binary encoding for integers and varints.
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

// Unmarshal decodes the given bytes into the given Packet object.
func Unmarshal(bytes []byte) (Packet, error) {
	return ReadFrom(coder.NewDecoder(bytes))
}
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

// pack encodes the given Packet object to header and data bytes.
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

// unpack decodes the given header and data bytes into the corresponding Packet object.
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
