package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/packet/coder"
)

type RequestPacket struct {
	Flags   uint8
	Serial  uint64
	Method  string
	Service string
	Headers map[string]string
	Body    []byte
}

func NewRequest() *RequestPacket {
	return &RequestPacket{}
}
func (p *RequestPacket) Type() PacketType {
	return REQUEST
}
func (p *RequestPacket) String() string {
	return fmt.Sprintf("RequestPacket{%s, %s, %d, %v, %d}", p.Method, p.Service, p.Flags, p.Headers, p.Serial)
}
func (p *RequestPacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*RequestPacket)
	return p.Method == o.Method &&
		p.Service == o.Service &&
		p.Flags == o.Flags &&
		p.Serial == o.Serial &&
		maps.Equal(p.Headers, o.Headers) &&
		bytes.Equal(p.Body, o.Body)
}
func (p *RequestPacket) EncodeTo(w coder.Writer) error {
	w.WriteUInt8(p.Flags)
	w.WriteUInt64(p.Serial)
	w.WriteString(p.Method)
	w.WriteString(p.Service)
	w.WriteStrMap(p.Headers)
	w.WriteBytes(p.Body)
	return nil
}
func (p *RequestPacket) DecodeFrom(r coder.Reader) error {
	var err error
	if p.Flags, err = r.ReadUInt8(); err != nil {
		return err
	}
	if p.Serial, err = r.ReadUInt64(); err != nil {
		return err
	}
	if p.Method, err = r.ReadString(); err != nil {
		return err
	}
	if p.Service, err = r.ReadString(); err != nil {
		return err
	}
	if p.Headers, err = r.ReadStrMap(); err != nil {
		return err
	}
	if p.Body, err = r.ReadAll(); err != nil {
		return err
	}
	return nil
}
