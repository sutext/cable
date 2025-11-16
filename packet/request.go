package packet

import (
	"bytes"
	"fmt"
	"maps"
	"math/rand/v2"

	"sutext.github.io/cable/coder"
)

type Request struct {
	ID      uint64
	Method  string
	Headers map[string]string
	Content []byte
}

func NewRequest(path string, content ...[]byte) *Request {
	var b []byte
	if len(content) > 0 {
		b = content[0]
	}
	return &Request{
		ID:      rand.Uint64(),
		Method:  path,
		Content: b,
	}
}
func (p *Request) Type() PacketType {
	return REQUEST
}
func (p *Request) String() string {
	return fmt.Sprintf("REQUEST(seq=%d, path=%s, body_len=%d)", p.ID, p.Method, len(p.Content))
}
func (p *Request) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Request)
	return p.ID == o.ID &&
		p.Method == o.Method &&
		maps.Equal(p.Headers, o.Headers) &&
		bytes.Equal(p.Content, o.Content)
}
func (p *Request) WriteTo(w coder.Encoder) error {
	w.WriteUInt64(p.ID)
	w.WriteString(p.Method)
	w.WriteStrMap(p.Headers)
	w.WriteBytes(p.Content)
	return nil
}
func (p *Request) ReadFrom(r coder.Decoder) error {
	var err error
	if p.ID, err = r.ReadUInt64(); err != nil {
		return err
	}
	if p.Method, err = r.ReadString(); err != nil {
		return err
	}
	if p.Headers, err = r.ReadStrMap(); err != nil {
		return err
	}
	if p.Content, err = r.ReadAll(); err != nil {
		return err
	}
	return nil
}
