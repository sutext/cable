// Package coder provides a binary encoding/decoding library for network communication.
// It supports various data types including basic types, variable-length integers,
// strings, byte arrays, and complex types like maps and slices.
package coder

import (
	"encoding/binary"
	"io"
)

// Encoder defines methods for encoding various data types into a binary buffer.
// The encoded data can be retrieved using the Bytes() method.
// All encoding methods use big-endian byte order for multi-byte values.
type Encoder interface {
	// Bytes returns the encoded binary data.
	Bytes() []byte
	// WriteBytes writes a slice of bytes directly to the buffer.
	WriteBytes(p []byte)
	// WriteUInt8 writes an 8-bit unsigned integer to the buffer.
	WriteUInt8(i uint8)
	// WriteUInt16 writes a 16-bit unsigned integer to the buffer in big-endian order.
	WriteUInt16(i uint16)
	// WriteUInt32 writes a 32-bit unsigned integer to the buffer in big-endian order.
	WriteUInt32(i uint32)
	// WriteUInt64 writes a 64-bit unsigned integer to the buffer in big-endian order.
	WriteUInt64(i uint64)
	// WriteBool writes a boolean value to the buffer (1 for true, 0 for false).
	WriteBool(b bool)
	// WriteInt8 writes an 8-bit signed integer to the buffer.
	WriteInt8(i int8)
	// WriteInt16 writes a 16-bit signed integer to the buffer in big-endian order.
	WriteInt16(i int16)
	// WriteInt32 writes a 32-bit signed integer to the buffer in big-endian order.
	WriteInt32(i int32)
	// WriteInt64 writes a 64-bit signed integer to the buffer in big-endian order.
	WriteInt64(i int64)
	// WriteVarint writes a variable-length integer to the buffer.
	// Uses Protocol Buffers varint encoding for efficient storage of small values.
	WriteVarint(i uint64)
	// WriteData writes a byte slice to the buffer with a varint length prefix.
	WriteData(data []byte)
	// WriteString writes a string to the buffer with a varint length prefix.
	WriteString(s string)
	// WriteStrMap writes a map[string]string to the buffer.
	// Format: [varint length] [key1] [value1] [key2] [value2] ...
	WriteStrMap(m map[string]string)
	// WriteStrings writes a slice of strings to the buffer.
	// Format: [varint length] [string1] [string2] ...
	WriteStrings(ss []string)
	// WriteUInt8Map writes a map[uint8]string to the buffer.
	// Format: [uint8 length] [key1] [value1] [key2] [value2] ...
	WriteUInt8Map(m map[uint8]string)
}

// Decoder defines methods for decoding binary data into various Go types.
// Implements io.Reader interface for compatibility with standard library functions.
// All decoding methods use big-endian byte order for multi-byte values.
type Decoder interface {
	io.Reader
	// ReadBytes reads exactly l bytes from the buffer.
	// Returns an error if there are not enough bytes remaining.
	ReadBytes(l uint64) ([]byte, error)
	// ReadUInt8 reads an 8-bit unsigned integer from the buffer.
	ReadUInt8() (uint8, error)
	// ReadUInt16 reads a 16-bit unsigned integer from the buffer in big-endian order.
	ReadUInt16() (uint16, error)
	// ReadUInt32 reads a 32-bit unsigned integer from the buffer in big-endian order.
	ReadUInt32() (uint32, error)
	// ReadUInt64 reads a 64-bit unsigned integer from the buffer in big-endian order.
	ReadUInt64() (uint64, error)
	// ReadBool reads a boolean value from the buffer (1 = true, 0 = false).
	ReadBool() (bool, error)
	// ReadInt8 reads an 8-bit signed integer from the buffer.
	ReadInt8() (int8, error)
	// ReadInt16 reads a 16-bit signed integer from the buffer in big-endian order.
	ReadInt16() (int16, error)
	// ReadInt32 reads a 32-bit signed integer from the buffer in big-endian order.
	ReadInt32() (int32, error)
	// ReadInt64 reads a 64-bit signed integer from the buffer in big-endian order.
	ReadInt64() (int64, error)
	// ReadVarint reads a variable-length integer from the buffer.
	// Uses Protocol Buffers varint encoding.
	ReadVarint() (uint64, error)
	// ReadData reads a byte slice from the buffer with a varint length prefix.
	ReadData() ([]byte, error)
	// ReadString reads a string from the buffer with a varint length prefix.
	ReadString() (string, error)
	// ReadStrMap reads a map[string]string from the buffer.
	// Expects format: [varint length] [key1] [value1] [key2] [value2] ...
	ReadStrMap() (map[string]string, error)
	// ReadStrings reads a slice of strings from the buffer.
	// Expects format: [varint length] [string1] [string2] ...
	ReadStrings() ([]string, error)
	// ReadUInt8Map reads a map[uint8]string from the buffer.
	// Expects format: [uint8 length] [key1] [value1] [key2] [value2] ...
	ReadUInt8Map() (map[uint8]string, error)
	// ReadAll reads all remaining bytes from the buffer.
	ReadAll() ([]byte, error)
}

// NewEncoder creates a new Encoder with an optional initial buffer capacity.
// If no capacity is provided, defaults to 256 bytes.
// The buffer will automatically grow as needed.
func NewEncoder(cap ...int) Encoder {
	if len(cap) > 0 && cap[0] > 0 {
		return &coder{pos: 0, buf: make([]byte, 0, cap[0])}
	}
	return &coder{pos: 0, buf: make([]byte, 0, 256)}
}

// NewDecoder creates a new Decoder that reads from the provided byte slice.
// The decoder maintains an internal position pointer that advances as data is read.
func NewDecoder(bytes []byte) Decoder {
	return &coder{pos: 0, buf: bytes}
}

// coder implements both Encoder and Decoder interfaces.
// Maintains a buffer for encoding/decoding and a position pointer for decoding.
type coder struct {
	pos uint64 // Current position in the buffer for reading
	buf []byte // Buffer containing encoded data
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
