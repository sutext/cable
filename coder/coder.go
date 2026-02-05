// Package coder provides a binary encoding/decoding library for network communication.
// It supports various data types including basic types, variable-length integers,
// strings, byte arrays, and complex types like maps and slices.
package coder

import (
	"encoding/binary"
	"slices"
)

// Encoder defines methods for encoding various data types into a binary buffer.
// The encoded data can be retrieved using the Bytes() method.
// All encoding methods use big-endian byte order for multi-byte values.
type Encoder interface {
	// Len returns the number of bytes written to the buffer.
	Len() int
	// Pick returns the encoded binary data.
	// Waring: After calling Pick(), the Encoder is no longer usable.
	Pick() []byte
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
func NewEncoder() Encoder {
	return _pool.get()
}

// NewDecoder creates a new Decoder that reads from the provided byte slice.
// The decoder maintains an internal position pointer that advances as data is read.
func NewDecoder(bytes []byte) Decoder {
	return &decoder{pos: 0, buf: bytes}
}

// encoder implements both Encoder and Decoder interfaces.
// Maintains a buffer for encoding/decoding and a position pointer for decoding.
type encoder struct {
	buf  []byte
	pool encPool
}

func (e *encoder) free() {
	e.buf = e.buf[:0]
	e.pool.put(e)
}
func (e *encoder) Len() int {
	return len(e.buf)
}
func (e *encoder) Pick() []byte {
	defer e.free()
	return slices.Clone(e.buf)
}

// Write bytes directly
func (e *encoder) WriteBytes(p []byte) {
	e.buf = append(e.buf, p...)
}

// Write UInt 8/16/32/64
func (e *encoder) WriteUInt8(i uint8) {
	e.buf = append(e.buf, i)
}
func (e *encoder) WriteUInt16(i uint16) {
	e.buf = binary.BigEndian.AppendUint16(e.buf, i)
}
func (e *encoder) WriteUInt32(i uint32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, i)
}
func (e *encoder) WriteUInt64(i uint64) {
	e.buf = binary.BigEndian.AppendUint64(e.buf, i)
}

func (e *encoder) WriteBool(bo bool) {
	if bo {
		e.WriteUInt8(1)
	} else {
		e.WriteUInt8(0)
	}
}

// Write Int 8/16/32/64
func (e *encoder) WriteInt8(i int8) {
	e.WriteUInt8(uint8(i))
}
func (e *encoder) WriteInt16(i int16) {
	e.WriteUInt16(uint16(i))
}
func (e *encoder) WriteInt32(i int32) {
	e.WriteUInt32(uint32(i))
}
func (e *encoder) WriteInt64(i int64) {
	e.WriteUInt64(uint64(i))
}

// Write Varint
func (e *encoder) WriteVarint(i uint64) {
	e.buf = binary.AppendUvarint(e.buf, i)
}

// Write Binary Data with Varint Length Prefix
func (e *encoder) WriteData(data []byte) {
	l := len(data)
	e.WriteVarint(uint64(l))
	if l > 0 {
		e.WriteBytes(data)
	}
}

// Write String with Varint Length Prefix
func (e *encoder) WriteString(s string) {
	bytes := []byte(s)
	e.WriteData(bytes)
}
func (e *encoder) WriteStrMap(m map[string]string) {
	e.WriteVarint(uint64(len(m)))
	for k, v := range m {
		e.WriteString(k)
		e.WriteString(v)
	}
}
func (e *encoder) WriteStrings(ss []string) {
	e.WriteVarint(uint64(len(ss)))
	for _, s := range ss {
		e.WriteString(s)
	}
}
func (e *encoder) WriteUInt8Map(m map[uint8]string) {
	e.WriteUInt8(uint8(len(m)))
	for k, v := range m {
		e.WriteUInt8(k)
		e.WriteString(v)
	}
}

type decoder struct {
	pos uint64
	buf []byte
}

// Read bytes directly
func (d *decoder) ReadBytes(l uint64) ([]byte, error) {
	if l == 0 {
		return nil, nil
	}
	if d.pos+l > uint64(len(d.buf)) {
		return nil, ErrBufferTooShort
	}
	p := d.buf[d.pos : d.pos+l]
	d.pos += l
	return p, nil
}

// Read UInt 8/16/32/64
func (d *decoder) ReadUInt8() (uint8, error) {
	if d.pos+1 > uint64(len(d.buf)) {
		return 0, ErrBufferTooShort
	}
	i := d.buf[d.pos]
	d.pos++
	return i, nil
}
func (d *decoder) ReadUInt16() (uint16, error) {
	bytes, err := d.ReadBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(bytes), nil
}

func (d *decoder) ReadUInt32() (uint32, error) {
	bytes, err := d.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(bytes), nil
}

func (d *decoder) ReadUInt64() (uint64, error) {
	bytes, err := d.ReadBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bytes), nil
}

func (d *decoder) ReadBool() (bool, error) {
	i, err := d.ReadUInt8()
	if err != nil {
		return false, err
	}
	return i == 1, nil
}
func (d *decoder) ReadInt8() (int8, error) {
	u, err := d.ReadUInt8()
	if err != nil {
		return 0, err
	}
	return int8(u), nil
}
func (d *decoder) ReadInt16() (int16, error) {
	u, err := d.ReadUInt16()
	if err != nil {
		return 0, err
	}
	return int16(u), nil
}
func (d *decoder) ReadInt32() (int32, error) {
	u, err := d.ReadUInt32()
	if err != nil {
		return 0, err
	}
	return int32(u), nil
}
func (d *decoder) ReadInt64() (int64, error) {
	u, err := d.ReadUInt64()
	if err != nil {
		return 0, err
	}
	return int64(u), nil
}

func (d *decoder) ReadVarint() (uint64, error) {
	varint, len := binary.Uvarint(d.buf[d.pos:])
	if len < 0 {
		return 0, ErrVarintOverflow
	}
	if len == 0 {
		return 0, ErrBufferTooShort
	}
	d.pos += uint64(len)
	return varint, nil
}
func (d *decoder) ReadString() (string, error) {
	if bytes, err := d.ReadData(); err != nil {
		return "", err
	} else {
		return string(bytes), nil
	}
}
func (d *decoder) ReadData() ([]byte, error) {
	l, err := d.ReadVarint()
	if err != nil {
		return nil, err
	}
	return d.ReadBytes(l)
}
func (d *decoder) ReadStrMap() (map[string]string, error) {
	l, err := d.ReadVarint()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for i := 0; i < int(l); i++ {
		k, err := d.ReadString()
		if err != nil {
			return nil, err
		}
		v, err := d.ReadString()
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}
func (d *decoder) ReadStrings() ([]string, error) {
	l, err := d.ReadVarint()
	if err != nil {
		return nil, err
	}
	ss := make([]string, l)
	for i := 0; i < int(l); i++ {
		s, err := d.ReadString()
		if err != nil {
			return nil, err
		}
		ss[i] = s
	}
	return ss, nil
}
func (d *decoder) ReadUInt8Map() (map[uint8]string, error) {
	l, err := d.ReadUInt8()
	if err != nil {
		return nil, err
	}
	m := make(map[uint8]string)
	for i := 0; i < int(l); i++ {
		k, err := d.ReadUInt8()
		if err != nil {
			return nil, err
		}
		v, err := d.ReadString()
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}
func (d *decoder) ReadAll() ([]byte, error) {
	l := uint64(len(d.buf))
	if d.pos >= l {
		return nil, nil
	}
	p := d.buf[d.pos:]
	d.pos = l
	return p, nil
}
