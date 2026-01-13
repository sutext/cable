package packet

import (
	"bytes"
	"fmt"
	"maps"
	"math/rand/v2"

	"sutext.github.io/cable/coder"
)

type Request struct {
	packet
	id     int64
	Method string
	Body   []byte
}

func NewRequest(method string, content ...[]byte) *Request {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Request{
		id:     rand.Int64(),
		Method: method,
		Body:   b,
	}
}
func (p *Request) ID() int64 {
	return p.id
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
		p.id == o.id &&
		p.Method == o.Method &&
		bytes.Equal(p.Body, o.Body)
}
func (p *Request) String() string {
	return fmt.Sprintf("REQUEST(ID=%d, Method=%s, Props=%v, Content=%d)", p.id, p.Method, p.props, len(p.Body))
}
func (p *Request) Response(code StatusCode, content ...[]byte) *Response {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Response{
		id:   p.id,
		Code: code,
		Body: b,
	}
}
func (p *Request) WriteTo(w coder.Encoder) error {
	w.WriteInt64(p.id)
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
	if p.id, err = r.ReadInt64(); err != nil {
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
