// Package coder provides a binary encoding/decoding library for network communication.
// It supports various data types including basic types, variable-length integers,
// strings, byte arrays, and complex types like maps and slices.
package coder

// Encodable defines an interface for types that can be encoded to a binary buffer.
// Implementations should write their state to the provided Encoder.
type Encodable interface {
	// WriteTo encodes the object to the provided Encoder.
	WriteTo(Encoder) error
}

// Decodable defines an interface for types that can be decoded from a binary buffer.
// Implementations should read their state from the provided Decoder.
type Decodable interface {
	// ReadFrom decodes the object from the provided Decoder.
	ReadFrom(Decoder) error
}

// Codable defines an interface that combines both Encodable and Decodable.
// Types implementing this interface can be both serialized and deserialized.
type Codable interface {
	Encodable
	Decodable
}

// Error defines error codes for the coder package.
type Error uint8

const (
	// ErrVarintOverflow is returned when decoding a varint that exceeds 64 bits.
	ErrVarintOverflow Error = 1
	// ErrBufferTooShort is returned when there are not enough bytes in the buffer to complete decoding.
	ErrBufferTooShort Error = 2
)

// Error returns the string representation of the error code.
func (e Error) Error() string {
	switch e {
	case ErrVarintOverflow:
		return "varint overflow"
	case ErrBufferTooShort:
		return "buffer too short"
	default:
		return "unknown error"
	}
}

// Marshal encodes an Encodable object into a binary byte slice.
// Returns the encoded bytes and any error encountered during encoding.
func Marshal(ec Encodable) ([]byte, error) {
	coder := NewEncoder()
	err := ec.WriteTo(coder)
	return coder.Bytes(), err
}

// Unmarshal decodes a binary byte slice into a Decodable object.
// Returns any error encountered during decoding.
func Unmarshal(b []byte, dc Decodable) error {
	coder := NewDecoder(b)
	return dc.ReadFrom(coder)
}
