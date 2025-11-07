package packet

import (
	"bytes"
	"fmt"
	"maps"
)

type Response struct {
	Serial  uint64
	Headers map[string]string
	Body    []byte
}

func NewResponse() *Response {
	return &Response{}
}
func (p *Response) Type() PacketType {
	return RESPONSE
}
func (p *Response) String() string {
	return fmt.Sprintf("Response{%v, %v, %d}", p.Serial, p.Headers, len(p.Body))
}
func (p *Response) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != RESPONSE {
		return false
	}
	o := other.(*Response)
	return p.Serial == o.Serial &&
		maps.Equal(p.Headers, o.Headers) &&
		bytes.Equal(p.Body, o.Body)
}

func (p *Response) EncodeTo(w Encoder) error {
	w.WriteUInt64(p.Serial)
	w.WriteStrMap(p.Headers)
	w.WriteBytes(p.Body)
	return nil
}
func (p *Response) DecodeFrom(r Decoder) error {
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
