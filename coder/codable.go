package coder

type Encodable interface {
	WriteTo(Encoder) error
}
type Decodable interface {
	ReadFrom(Decoder) error
}
type Codable interface {
	Encodable
	Decodable
}
type Error uint8

const (
	ErrVarintOverflow Error = 1
	ErrBufferTooShort Error = 2
)

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

func Marshal(ec Encodable) ([]byte, error) {
	coder := NewEncoder()
	err := ec.WriteTo(coder)
	return coder.Bytes(), err
}

func Unmarshal(b []byte, dc Decodable) error {
	coder := NewDecoder(b)
	return dc.ReadFrom(coder)
}
