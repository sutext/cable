package packet

import (
	"bytes"
	"fmt"
	"maps"
	"math/rand/v2"
)

type Request struct {
	Seq     uint64
	Path    string
	Body    []byte
	headers map[string]string
}

func NewRequest(path string, body ...[]byte) *Request {
	var b []byte
	if len(body) > 0 {
		b = body[0]
	}
	return &Request{
		Seq:  rand.Uint64(),
		Path: path,
		Body: b,
	}
}
func (p *Request) Headers() map[string]string {
	return p.headers
}
func (p *Request) SetHeader(key, value string) {
	if p.headers == nil {
		p.headers = make(map[string]string)
	}
	p.headers[key] = value
}
func (p *Request) GetHeader(key string) (string, bool) {
	if p.headers == nil {
		return "", false
	}
	value, ok := p.headers[key]
	return value, ok
}
func (p *Request) Type() PacketType {
	return REQUEST
}
func (p *Request) String() string {
	return fmt.Sprintf("REQUEST(seq=%d, path=%s, body_len=%d)", p.Seq, p.Path, len(p.Body))
}
func (p *Request) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if p.Type() != other.Type() {
		return false
	}
	o := other.(*Request)
	return p.Seq == o.Seq &&
		p.Path == o.Path &&
		maps.Equal(p.headers, o.headers) &&
		bytes.Equal(p.Body, o.Body)
}
func (p *Request) EncodeTo(w Encoder) error {
	w.WriteUInt64(p.Seq)
	w.WriteString(p.Path)
	w.WriteStrMap(p.headers)
	w.WriteBytes(p.Body)
	return nil
}
func (p *Request) DecodeFrom(r Decoder) error {
	var err error
	if p.Seq, err = r.ReadUInt64(); err != nil {
		return err
	}
	if p.Path, err = r.ReadString(); err != nil {
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
