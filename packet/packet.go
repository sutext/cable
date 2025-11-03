package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"sutext.github.io/cable/internal/buffer"
)

const (
	MIN_LEN int = 0
	MID_LEN int = 0xfff
	MAX_LEN int = 0xfffffff
)

type PacketType uint8

const (
	CONNECT PacketType = 1 // CONNECT packet
	CONNACK PacketType = 2 // CONNACK packet
	PING    PacketType = 3 // PING packet
	PONG    PacketType = 4 // PONG packet
	DATA    PacketType = 5 // DATA packet
	CLOSE   PacketType = 6 // CLOSE packet
)

var (
	ErrUnkownPacketType   = errors.New("unknown packet type")
	ErrPacketSizeTooLarge = errors.New("packet size too large")
)

func (t PacketType) String() string {
	switch t {
	case CONNECT:
		return "CONNECT"
	case CONNACK:
		return "CONNACK"
	case PING:
		return "PING"
	case PONG:
		return "PONG"
	case DATA:
		return "DATA"
	case CLOSE:
		return "CLOSE"
	default:
		return "UNKNOWN"
	}
}

type Packet interface {
	fmt.Stringer
	buffer.WriteTo
	buffer.ReadFrom
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
func (p *pingpong) WriteTo(w *buffer.Buffer) error {
	return nil
}
func (p *pingpong) ReadFrom(r *buffer.Buffer) error {
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
	flag := header[0] & 0x10
	var length uint32
	if flag != 0 {
		bs := make([]byte, 2)
		io.ReadFull(r, bs)
		length = binary.BigEndian.Uint32([]byte{header[0] & 0x0f, header[1], bs[0], bs[1]})
	} else {
		length = binary.BigEndian.Uint32([]byte{0, 0, header[0] & 0x0f, header[1]})
	}
	// read data
	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return nil, err
	}
	buf := buffer.New(data)
	switch packetType {
	case CONNECT:
		conn := &ConnectPacket{}
		err := conn.ReadFrom(buf)
		if err != nil {
			return nil, err
		}
		return conn, nil
	case CONNACK:
		connack := &ConnackPacket{}
		err := connack.ReadFrom(buf)
		if err != nil {
			return nil, err
		}
		return connack, nil
	case DATA:
		data := &DataPacket{}
		err := data.ReadFrom(buf)
		if err != nil {
			return nil, err
		}
		return data, nil
	case PING:
		return NewPing(), nil
	case PONG:
		return NewPong(), nil
	case CLOSE:
		close := &ClosePacket{}
		err := close.ReadFrom(buf)
		if err != nil {
			return nil, err
		}
		return close, nil
	default:
		return nil, ErrUnkownPacketType
	}
}
func WriteTo(w io.Writer, p Packet) error {
	buf := buffer.New()
	err := p.WriteTo(buf)
	if err != nil {
		return err
	}
	length := buf.Len()
	if length > MAX_LEN {
		return ErrPacketSizeTooLarge
	}
	var header []byte
	if length > MID_LEN {
		header = make([]byte, 4)
		binary.BigEndian.PutUint32(header, uint32(length))
		header[0] = byte(p.Type()<<5) | 0x10 | header[0]
	} else {
		header = make([]byte, 2)
		binary.BigEndian.PutUint16(header, uint16(length))
		header[0] = byte(p.Type()<<5) | header[0]
	}
	_, err = w.Write(header)
	if err != nil {
		return err
	}
	_, err = w.Write(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}
