package packet

import (
	"bytes"
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type Response struct {
	ID      int64
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
	return p.ID == o.ID &&
		bytes.Equal(p.Content, o.Content) &&
		maps.Equal(p.Headers, o.Headers)
}

func (p *Response) WriteTo(w coder.Encoder) error {
	w.WriteInt64(p.ID)
	w.WriteStrMap(p.Headers)
	w.WriteBytes(p.Content)
	return nil
}
func (p *Response) ReadFrom(r coder.Decoder) error {
	var err error
	if p.ID, err = r.ReadInt64(); err != nil {
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
