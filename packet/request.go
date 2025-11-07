package packet

import (
	"bytes"
	"fmt"
	"maps"
)

type Request struct {
	Flags   uint8
	Serial  uint64
	Method  string
	Service string
	Headers map[string]string
	Body    []byte
}

func NewRequest() *Request {
	return &Request{}
}
func (p *Request) Type() PacketType {
	return REQUEST
}
func (p *Request) String() string {
	return fmt.Sprintf("Request{%s, %s, %d, %v, %d}", p.Method, p.Service, p.Flags, p.Headers, p.Serial)
}
func (p *Request) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Request)
	return p.Method == o.Method &&
		p.Service == o.Service &&
		p.Flags == o.Flags &&
		p.Serial == o.Serial &&
		maps.Equal(p.Headers, o.Headers) &&
		bytes.Equal(p.Body, o.Body)
}
func (p *Request) EncodeTo(w Encoder) error {
	w.WriteUInt8(p.Flags)
	w.WriteUInt64(p.Serial)
	w.WriteString(p.Method)
	w.WriteString(p.Service)
	w.WriteStrMap(p.Headers)
	w.WriteBytes(p.Body)
	return nil
}
func (p *Request) DecodeFrom(r Decoder) error {
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
