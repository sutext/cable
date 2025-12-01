package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type ResponseCode uint8

const (
	OK         ResponseCode = 0
	NotFound   ResponseCode = 100
	Forbidden  ResponseCode = 101
	BadRequest ResponseCode = 255
)

type Response struct {
	packet
	ID      int64
	Code    ResponseCode
	Headers map[string]string
	Content []byte
}

func NewResponse(id int64, content ...[]byte) *Response {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Response{
		ID:      id,
		Content: b,
	}
}
func (p *Response) Type() PacketType {
	return RESPONSE
}
func (p *Response) String() string {
	return fmt.Sprintf("RESPONSE(id=%d, content=%d)", p.ID, len(p.Content))
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
		p.ID == o.ID &&
		p.Code == o.Code &&
		maps.Equal(p.Headers, o.Headers) &&
		bytes.Equal(p.Content, o.Content)
}

func (p *Response) WriteTo(w coder.Encoder) error {
	w.WriteInt64(p.ID)
	w.WriteUInt8(uint8(p.Code))
	w.WriteStrMap(p.Headers)
	if err := p.packet.WriteTo(w); err != nil {
		return err
	}
	w.WriteBytes(p.Content)
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
	headers, err := r.ReadStrMap()
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
	p.ID = id
	p.Code = ResponseCode(code)
	p.Headers = headers
	p.Content = b
	return nil
}
