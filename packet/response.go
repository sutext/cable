package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type ResponseCode uint8

const (
	RequestOK        ResponseCode = 0
	RequestNotFound  ResponseCode = 100
	RequestForbidden ResponseCode = 101
	BadRequest       ResponseCode = 255
)

type Response struct {
	packet
	id   int64
	Code ResponseCode
	Body []byte
}

func NewResponse(id int64, content ...[]byte) *Response {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Response{
		id:   id,
		Body: b,
	}
}
func (p *Response) ID() int64 {
	return p.id
}
func (p *Response) Type() PacketType {
	return RESPONSE
}
func (p *Response) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != RESPONSE {
		return false
	}
	o := other.(*Response)
	return maps.Equal(p.props, o.props) &&
		p.id == o.id &&
		p.Code == o.Code &&
		bytes.Equal(p.Body, o.Body)
}
func (p *Response) String() string {
	return fmt.Sprintf("RESPONSE(ID=%d, Code=%d,  Props=%v, Content=%d)", p.id, p.Code, p.props, len(p.Body))
}
func (p *Response) WriteTo(w coder.Encoder) error {
	w.WriteInt64(p.id)
	w.WriteUInt8(uint8(p.Code))
	if err := p.packet.WriteTo(w); err != nil {
		return err
	}
	w.WriteBytes(p.Body)
	return nil
}
func (p *Response) ReadFrom(r coder.Decoder) error {
	id, err := r.ReadInt64()
	if err != nil {
		return err
	}
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	if err := p.packet.ReadFrom(r); err != nil {
		return err
	}
	b, err := r.ReadAll()
	if err != nil {
		return err
	}
	p.id = id
	p.Code = ResponseCode(code)
	p.Body = b
	return nil
}
