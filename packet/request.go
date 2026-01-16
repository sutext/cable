package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type Request struct {
	packet
	ID     uint16
	Method string
	Body   []byte
}

func NewRequest(method string, body ...[]byte) *Request {
	var b []byte
	if len(body) > 0 {
		b = body[0]
	}
	return &Request{
		Method: method,
		Body:   b,
	}
}
func (p *Request) Type() PacketType {
	return REQUEST
}
func (p *Request) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Request)
	return maps.Equal(p.props, o.props) &&
		p.ID == o.ID &&
		p.Method == o.Method &&
		bytes.Equal(p.Body, o.Body)
}
func (p *Request) String() string {
	return fmt.Sprintf("REQUEST(ID=%d, Method=%s, Props=%v, Content=%d)", p.ID, p.Method, p.props, len(p.Body))
}
func (p *Request) Response(code StatusCode, content ...[]byte) *Response {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Response{
		id:   p.ID,
		Code: code,
		Body: b,
	}
}
func (p *Request) WriteTo(w coder.Encoder) error {
	w.WriteUInt16(p.ID)
	w.WriteString(p.Method)
	err := p.packet.WriteTo(w)
	if err != nil {
		return err
	}
	w.WriteBytes(p.Body)
	return nil
}
func (p *Request) ReadFrom(r coder.Decoder) error {
	var err error
	if p.ID, err = r.ReadUInt16(); err != nil {
		return err
	}
	if p.Method, err = r.ReadString(); err != nil {
		return err
	}
	if err = p.packet.ReadFrom(r); err != nil {
		return err
	}
	if p.Body, err = r.ReadAll(); err != nil {
		return err
	}
	return nil
}
