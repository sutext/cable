package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/packet/coder"
)

type ResponsePacket struct {
	Serial  uint64
	Headers map[string]string
	Body    []byte
}

func NewResponse() *ResponsePacket {
	return &ResponsePacket{}
}
func (p *ResponsePacket) Type() PacketType {
	return RESPONSE
}
func (p *ResponsePacket) String() string {
	return fmt.Sprintf("ResponsePacket{%v, %v, %d}", p.Serial, p.Headers, len(p.Body))
}
func (p *ResponsePacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != RESPONSE {
		return false
	}
	o := other.(*ResponsePacket)
	return p.Serial == o.Serial &&
		maps.Equal(p.Headers, o.Headers) &&
		bytes.Equal(p.Body, o.Body)
}

func (p *ResponsePacket) EncodeTo(w coder.Writer) error {
	w.WriteUInt64(p.Serial)
	w.WriteStrMap(p.Headers)
	w.WriteBytes(p.Body)
	return nil
}
func (p *ResponsePacket) DecodeFrom(r coder.Reader) error {
	var err error
	if p.Serial, err = r.ReadUInt64(); err != nil {
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
