package coder

import (
	"encoding/binary"
	"io"
)

type Encoder interface {
	Bytes() []byte
	WriteBytes(p []byte)
	WriteUInt8(i uint8)
	WriteUInt16(i uint16)
	WriteUInt32(i uint32)
	WriteUInt64(i uint64)
	WriteBool(b bool)
	WriteInt8(i int8)
	WriteInt16(i int16)
	WriteInt32(i int32)
	WriteInt64(i int64)
	WriteVarint(i uint64)
	WriteData(data []byte)
	WriteString(s string)
	WriteStrMap(m map[string]string)
	WriteStrings(ss []string)
	WriteUInt8Map(m map[uint8]string)
}
type Decoder interface {
	io.Reader
	ReadBytes(l uint64) ([]byte, error)
	ReadUInt8() (uint8, error)
	ReadUInt16() (uint16, error)
	ReadUInt32() (uint32, error)
	ReadUInt64() (uint64, error)
	ReadBool() (bool, error)
	ReadInt8() (int8, error)
	ReadInt16() (int16, error)
	ReadInt32() (int32, error)
	ReadInt64() (int64, error)
	ReadVarint() (uint64, error)
	ReadData() ([]byte, error)
	ReadString() (string, error)
	ReadStrMap() (map[string]string, error)
	ReadStrings() ([]string, error)
	ReadUInt8Map() (map[uint8]string, error)
	ReadAll() ([]byte, error)
}

func NewEncoder(cap ...int) Encoder {
	if len(cap) > 0 && cap[0] > 0 {
		return &coder{pos: 0, buf: make([]byte, 0, cap[0])}
	}
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

func (b *coder) WriteBool(bo bool) {
	if bo {
		b.WriteUInt8(1)
	} else {
		b.WriteUInt8(0)
	}
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
func (b *coder) WriteStrings(ss []string) {
	b.WriteVarint(uint64(len(ss)))
	for _, s := range ss {
		b.WriteString(s)
	}
}
func (b *coder) WriteUInt8Map(m map[uint8]string) {
	b.WriteUInt8(uint8(len(m)))
	for k, v := range m {
		b.WriteUInt8(k)
		b.WriteString(v)
	}
}

// implement io.Reader interface
func (b *coder) Read(p []byte) (int, error) {
	l := len(p)
	n, err := b.ReadBytes(uint64(l))
	if err != nil {
		return 0, err
	}
	copy(p, n)
	return len(p), nil
}

// Read bytes directly
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

// Read UInt 8/16/32/64
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

func (b *coder) ReadBool() (bool, error) {
	i, err := b.ReadUInt8()
	if err != nil {
		return false, err
	}
	return i == 1, nil
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
func (b *coder) ReadStrings() ([]string, error) {
	l, err := b.ReadVarint()
	if err != nil {
		return nil, err
	}
	ss := make([]string, l)
	for i := 0; i < int(l); i++ {
		s, err := b.ReadString()
		if err != nil {
			return nil, err
		}
		ss[i] = s
	}
	return ss, nil
}
func (b *coder) ReadUInt8Map() (map[uint8]string, error) {
	l, err := b.ReadUInt8()
	if err != nil {
		return nil, err
	}
	m := make(map[uint8]string)
	for i := 0; i < int(l); i++ {
		k, err := b.ReadUInt8()
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
