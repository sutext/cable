package packet

import (
	"encoding/binary"
)

type Encoder interface {
	Bytes() []byte
	WriteBytes(p []byte)
	WriteUInt8(i uint8)
	WriteUInt16(i uint16)
	WriteUInt32(i uint32)
	WriteUInt64(i uint64)
	WriteInt8(i int8)
	WriteInt16(i int16)
	WriteInt32(i int32)
	WriteInt64(i int64)
	WriteVarint(i uint64)
	WriteData(data []byte)
	WriteString(s string)
	WriteStrMap(m map[string]string)
}
type Decoder interface {
	ReadBytes(l uint64) ([]byte, error)
	ReadUInt8() (uint8, error)
	ReadUInt16() (uint16, error)
	ReadUInt32() (uint32, error)
	ReadUInt64() (uint64, error)
	ReadInt8() (int8, error)
	ReadInt16() (int16, error)
	ReadInt32() (int32, error)
	ReadInt64() (int64, error)
	ReadVarint() (uint64, error)
	ReadData() ([]byte, error)
	ReadString() (string, error)
	ReadStrMap() (map[string]string, error)
	ReadAll() ([]byte, error)
}

func NewEncoder() Encoder {
	return &coder{pos: 0, buf: make([]byte, 0, 256)}
}
func NewDecoder(bytes []byte) Decoder {
	return &coder{pos: 0, buf: bytes}
}

type coder struct {
	pos uint64
	buf []byte
}

func (b *coder) Bytes() []byte {
	return b.buf
}

// Write bytes directly
func (b *coder) WriteBytes(p []byte) {
	b.buf = append(b.buf, p...)
}

// Write UInt 8/16/32/64
func (b *coder) WriteUInt8(i uint8) {
	b.buf = append(b.buf, i)
}
func (b *coder) WriteUInt16(i uint16) {
	b.buf = binary.BigEndian.AppendUint16(b.buf, i)
}
func (b *coder) WriteUInt32(i uint32) {
	b.buf = binary.BigEndian.AppendUint32(b.buf, i)
}
func (b *coder) WriteUInt64(i uint64) {
	b.buf = binary.BigEndian.AppendUint64(b.buf, i)
}

// Write Int 8/16/32/64
func (b *coder) WriteInt8(i int8) {
	b.WriteUInt8(uint8(i))
}
func (b *coder) WriteInt16(i int16) {
	b.WriteUInt16(uint16(i))
}
func (b *coder) WriteInt32(i int32) {
	b.WriteUInt32(uint32(i))
}
func (b *coder) WriteInt64(i int64) {
	b.WriteUInt64(uint64(i))
}

// Write Varint
func (b *coder) WriteVarint(i uint64) {
	b.buf = binary.AppendUvarint(b.buf, i)
}

// Write Binary Data with Varint Length Prefix
func (b *coder) WriteData(data []byte) {
	l := len(data)
	b.WriteVarint(uint64(l))
	if l > 0 {
		b.WriteBytes(data)
	}
}

// Write String with Varint Length Prefix
func (b *coder) WriteString(s string) {
	bytes := []byte(s)
	b.WriteData(bytes)
}
func (b *coder) WriteStrMap(m map[string]string) {
	b.WriteVarint(uint64(len(m)))
	for k, v := range m {
		b.WriteString(k)
		b.WriteString(v)
	}
}

func (b *coder) ReadBytes(l uint64) ([]byte, error) {
	if l == 0 {
		return nil, nil
	}
	if b.pos+l > uint64(len(b.buf)) {
		return nil, ErrBufferTooShort
	}
	p := b.buf[b.pos : b.pos+l]
	b.pos += l
	return p, nil
}
func (b *coder) ReadUInt8() (uint8, error) {
	if b.pos+1 > uint64(len(b.buf)) {
		return 0, ErrBufferTooShort
	}
	i := b.buf[b.pos]
	b.pos++
	return i, nil
}
func (b *coder) ReadUInt16() (uint16, error) {
	bytes, err := b.ReadBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(bytes), nil
}

func (b *coder) ReadUInt32() (uint32, error) {
	bytes, err := b.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(bytes), nil
}

func (b *coder) ReadUInt64() (uint64, error) {
	bytes, err := b.ReadBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bytes), nil
}
func (b *coder) ReadInt8() (int8, error) {
	u, err := b.ReadUInt8()
	if err != nil {
		return 0, err
	}
	return int8(u), nil
}
func (b *coder) ReadInt16() (int16, error) {
	u, err := b.ReadUInt16()
	if err != nil {
		return 0, err
	}
	return int16(u), nil
}
func (b *coder) ReadInt32() (int32, error) {
	u, err := b.ReadUInt32()
	if err != nil {
		return 0, err
	}
	return int32(u), nil
}
func (b *coder) ReadInt64() (int64, error) {
	u, err := b.ReadUInt64()
	if err != nil {
		return 0, err
	}
	return int64(u), nil
}

func (b *coder) ReadVarint() (uint64, error) {
	varint, len := binary.Uvarint(b.buf[b.pos:])
	if len < 0 {
		return 0, ErrVarintOverflow
	}
	if len == 0 {
		return 0, ErrBufferTooShort
	}
	b.pos += uint64(len)
	return varint, nil
}
func (b *coder) ReadString() (string, error) {
	if bytes, err := b.ReadData(); err != nil {
		return "", err
	} else {
		return string(bytes), nil
	}
}
func (b *coder) ReadData() ([]byte, error) {
	l, err := b.ReadVarint()
	if err != nil {
		return nil, err
	}
	return b.ReadBytes(l)
}
func (b *coder) ReadStrMap() (map[string]string, error) {
	l, err := b.ReadVarint()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for i := 0; i < int(l); i++ {
		k, err := b.ReadString()
		if err != nil {
			return nil, err
		}
		v, err := b.ReadString()
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}
func (b *coder) ReadAll() ([]byte, error) {
	l := uint64(len(b.buf))
	if b.pos >= l {
		return nil, nil
	}
	p := b.buf[b.pos:]
	b.pos = l
	return p, nil
}

// Marshal encodes the given Packet object to bytes.
// Use compact binary encoding for integers and varints.
// NOTE: Marshal does not contain any header or length information.
func Marshal(p Packet) ([]byte, error) {
	var buf []byte
	switch p.Type() {
	case PING, PONG:
		return nil, nil
	case CLOSE, CONNACK:
		buf = make([]byte, 1)
	case CONNECT, REQUEST, RESPONSE, MESSAGE:
		buf = make([]byte, 0, 256)
	}
	coder := &coder{pos: 0, buf: buf}
	if err := p.EncodeTo(coder); err != nil {
		return nil, err
	}
	return coder.buf, nil
}

// Unmarshal decodes the given bytes into the given Packet object.
// NOTE: Unmarshal assumes that the bytes contain a complete packet.
func Unmarshal(p Packet, bytes []byte) error {
	return p.DecodeFrom(&coder{pos: 0, buf: bytes})
}
