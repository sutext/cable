package packet

import (
	"bytes"
	"fmt"
	"maps"
)

type Response struct {
	Seq     uint64
	Body    []byte
	headers map[string]string
}

func NewResponse(seq uint64, body ...[]byte) *Response {
	var b []byte
	if len(body) > 0 {
		b = body[0]
	}
	return &Response{
		Seq:  seq,
		Body: b,
	}
}
func (p *Response) Headers() map[string]string {
	return p.headers
}
func (p *Response) SetHeader(key, value string) {
	if p.headers == nil {
		p.headers = make(map[string]string)
	}
	p.headers[key] = value
}
func (p *Response) GetHeader(key string) (string, bool) {
	if p.headers == nil {
		return "", false
	}
	value, ok := p.headers[key]
	return value, ok
}
func (p *Response) Type() PacketType {
	return RESPONSE
}
func (p *Response) String() string {
	return fmt.Sprintf("RESPONSE(seq=%d, body_len=%d)", p.Seq, len(p.Body))
}
func (p *Response) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != RESPONSE {
		return false
	}
	o := other.(*Response)
	return p.Seq == o.Seq &&
		bytes.Equal(p.Body, o.Body) &&
		maps.Equal(p.headers, o.headers)
}

func (p *Response) EncodeTo(w Encoder) error {
	w.WriteUInt64(p.Seq)
	w.WriteStrMap(p.headers)
	w.WriteBytes(p.Body)
	return nil
}
func (p *Response) DecodeFrom(r Decoder) error {
	var err error
	if p.Seq, err = r.ReadUInt64(); err != nil {
		return err
	}
	if p.headers, err = r.ReadStrMap(); err != nil {
		return err
	}
	if p.Body, err = r.ReadAll(); err != nil {
		return err
	}
	return nil
}
