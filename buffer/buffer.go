package buffer

import (
	"encoding/binary"
)

type Error uint8

func (e Error) Error() string {
	switch e {
	case ErrBufferTooShort:
		return "buffer too short"
	case ErrVarintOverflow:
		return "varint overflow"
	default:
		return "unknown error"
	}
}

const (
	ErrBufferTooShort Error = 1
	ErrVarintOverflow Error = 2
)

type WriteTo interface {
	WriteTo(buf *Buffer) error
}
type ReadFrom interface {
	ReadFrom(buf *Buffer) error
}
type Buffer struct {
	pos int
	buf []byte
}

func New(buf ...[]byte) *Buffer {
	switch len(buf) {
	case 0:
		return &Buffer{pos: 0, buf: []byte{}}
	case 1:
		return &Buffer{pos: 0, buf: buf[0]}
	default:
		panic("too many arguments")
	}
}
func (b *Buffer) Len() int {
	return len(b.buf)
}
func (b *Buffer) Cap() int {
	return cap(b.buf)
}
func (b *Buffer) Bytes() []byte {
	return b.buf
}
func (b *Buffer) WriteBytes(p []byte) {
	b.buf = append(b.buf, p...)
}

// Write UInt
func (b *Buffer) WriteUInt8(i uint8) {
	b.buf = append(b.buf, i)
}
func (b *Buffer) WriteUInt16(i uint16) {
	b.buf = binary.BigEndian.AppendUint16(b.buf, i)
}
func (b *Buffer) WriteUInt32(i uint32) {
	b.buf = binary.BigEndian.AppendUint32(b.buf, i)
}
func (b *Buffer) WriteUInt64(i uint64) {
	b.buf = binary.BigEndian.AppendUint64(b.buf, i)
}

// Write Int
func (b *Buffer) WriteInt8(i int8) {
	b.WriteUInt8(uint8(i))
}
func (b *Buffer) WriteInt16(i int16) {
	b.WriteUInt16(uint16(i))
}
func (b *Buffer) WriteInt32(i int32) {
	b.WriteUInt32(uint32(i))
}
func (b *Buffer) WriteInt64(i int64) {
	b.WriteUInt64(uint64(i))
}

// Write Varint
func (b *Buffer) WriteVarint(i int64) {
	b.buf = binary.AppendVarint(b.buf, i)
}
func (b *Buffer) WriteString(s string) {
	bytes := []byte(s)
	b.WriteVarint(int64(len(bytes)))
	b.WriteBytes(bytes)
}
func (b *Buffer) WriteData(data []byte) {
	b.WriteVarint(int64(len(data)))
	b.WriteBytes(data)
}
func (b *Buffer) ReadBytes(l int) ([]byte, error) {
	if b.pos+l > len(b.buf) {
		return nil, ErrBufferTooShort
	}
	p := b.buf[b.pos : b.pos+l]
	b.pos += l
	return p, nil
}
func (b *Buffer) ReadUInt8() (uint8, error) {
	if b.pos+1 > len(b.buf) {
		return 0, ErrBufferTooShort
	}
	i := b.buf[b.pos]
	b.pos++
	return i, nil
}
func (b *Buffer) ReadUInt16() (uint16, error) {
	bytes, err := b.ReadBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(bytes), nil
}

func (b *Buffer) ReadUInt32() (uint32, error) {
	bytes, err := b.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(bytes), nil
}

func (b *Buffer) ReadUInt64() (uint64, error) {
	bytes, err := b.ReadBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bytes), nil
}
func (b *Buffer) ReadInt8() (int8, error) {
	u, err := b.ReadUInt8()
	if err != nil {
		return 0, err
	}
	return int8(u), nil
}
func (b *Buffer) ReadInt16() (int16, error) {
	u, err := b.ReadUInt16()
	if err != nil {
		return 0, err
	}
	return int16(u), nil
}
func (b *Buffer) ReadInt32() (int32, error) {
	u, err := b.ReadUInt32()
	if err != nil {
		return 0, err
	}
	return int32(u), nil
}
func (b *Buffer) ReadInt64() (int64, error) {
	u, err := b.ReadUInt64()
	if err != nil {
		return 0, err
	}
	return int64(u), nil
}

func (b *Buffer) ReadVarint() (int64, error) {
	varint, len := binary.Varint(b.buf[b.pos:])
	if len < 0 {
		return 0, ErrVarintOverflow
	}
	if len == 0 {
		return 0, ErrBufferTooShort
	}
	b.pos += len
	return varint, nil
}
func (b *Buffer) ReadString() (string, error) {
	l, err := b.ReadVarint()
	if err != nil {
		return "", err
	}
	bytes, err := b.ReadBytes(int(l))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
func (b *Buffer) ReadData() ([]byte, error) {
	l, err := b.ReadVarint()
	if err != nil {
		return nil, err
	}
	return b.ReadBytes(int(l))
}
func (b *Buffer) ReadAll() ([]byte, error) {
	if b.pos >= len(b.buf) {
		return nil, ErrBufferTooShort
	}
	p := b.buf[b.pos:]
	b.pos = len(b.buf)
	return p, nil
}
