package packet

import (
	"bufio"
	"fmt"
	"io"
	"slices"
)

const (
	MIN_LEN int = 0
	MID_LEN int = 0x7ff        // 2047
	MAX_LEN int = 0x7ff_ffffff // 32GB
)

// Error represents an error code.
type Error uint8

const (
	ErrBufferTooShort     Error = 1
	ErrVarintOverflow     Error = 2
	ErrInvalidReadLen     Error = 3
	ErrUnknownPacketType  Error = 4
	ErrPacketSizeTooLarge Error = 5
)

func (e Error) Error() string {
	switch e {
	case ErrBufferTooShort:
		return "buffer too short"
	case ErrVarintOverflow:
		return "varint overflow"
	case ErrInvalidReadLen:
		return "invalid length when read"
	case ErrUnknownPacketType:
		return "unknown packet type"
	case ErrPacketSizeTooLarge:
		return "packet size too large"
	default:
		return "unknown error"
	}
}

// PacketType represents a packet type.
type PacketType uint8

const (
	CONNECT  PacketType = iota // CONNECT packet
	CONNACK                    // CONNACK packet
	MESSAGE                    // MESSAGE packet
	REQUEST                    // REQUEST packet
	RESPONSE                   // RESPONSE packet
	PING                       // PING packet
	PONG                       // PONG packet
	CLOSE                      // CLOSE packet
)

func (t PacketType) String() string {
	switch t {
	case CONNECT:
		return "CONNECT"
	case CONNACK:
		return "CONNACK"
	case MESSAGE:
		return "MESSAGE"
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

type Packet interface {
	fmt.Stringer
	Codable
	Type() PacketType
	Equal(Packet) bool
}

type pingpong struct {
	t PacketType
}

func NewPing() Packet {
	return &pingpong{t: PING}
}
func NewPong() Packet {
	return &pingpong{t: PONG}
}
func (p *pingpong) Type() PacketType {
	return p.t
}
func (p *pingpong) String() string {
	return p.t.String()
}
func (p *pingpong) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	return p.t == other.Type()
}
func (p *pingpong) EncodeTo(c Encoder) error {
	return nil
}
func (p *pingpong) DecodeFrom(c Decoder) error {
	return nil
}
func ReadFrom(r io.Reader) (Packet, error) {
	// read header
	header := make([]byte, 2)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return nil, err
	}
	packetType := PacketType(header[0] >> 5)
	//read length
	byteCount := (header[0] >> 3) & 0x03
	length := uint64(header[0]&0x07)<<8 | uint64(header[1])
	if byteCount > 0 {
		bs := make([]byte, byteCount)
		io.ReadFull(r, bs)
		for _, b := range bs {
			length = length<<8 | uint64(b)
		}
	}
	// read data
	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return nil, err
	}
	switch packetType {
	case CONNECT:
		conn := &Connect{}
		if err := Unmarshal(conn, data); err != nil {
			return nil, err
		}
		return conn, nil
	case CONNACK:
		connack := &Connack{}
		if err := Unmarshal(connack, data); err != nil {
			return nil, err
		}
		return connack, nil
	case MESSAGE:
		msg := &Message{}
		if err := Unmarshal(msg, data); err != nil {
			return nil, err
		}
		return msg, nil
	case REQUEST:
		req := &Request{}
		if err := Unmarshal(req, data); err != nil {
			return nil, err
		}
		return req, nil
	case RESPONSE:
		res := &Response{}
		if err := Unmarshal(res, data); err != nil {
			return nil, err
		}
		return res, nil
	case PING:
		return NewPing(), nil
	case PONG:
		return NewPong(), nil
	case CLOSE:
		close := &Close{}
		if err := Unmarshal(close, data); err != nil {
			return nil, err
		}
		return close, nil
	default:
		return nil, ErrUnknownPacketType
	}
}
func WriteTo(w io.Writer, p Packet) error {
	bw := bufio.NewWriter(w)
	data, err := Marshal(p)
	if err != nil {
		return err
	}
	length := len(data)
	if length > MAX_LEN {
		return ErrPacketSizeTooLarge
	}
	var header []byte
	if length > MID_LEN {
		bs := make([]byte, 0, 5)
		for length > 0 {
			bs = append(bs, byte(length&0xff))
			length >>= 8
		}
		slices.Reverse(bs)
		if bs[0] > 7 {
			header = make([]byte, len(bs)+1)
			copy(header[1:], bs)
		} else {
			header = bs
		}
		header[0] = byte(p.Type()<<5) | byte(len(header)-2)<<3 | header[0]
	} else {
		header = make([]byte, 2)
		header[0] = byte(p.Type()<<5) | byte(length>>8)
		header[1] = byte(length)
	}
	_, err = bw.Write(header)
	if err != nil {
		return err
	}
	_, err = bw.Write(data)
	if err != nil {
		return err
	}
	return bw.Flush()
}
