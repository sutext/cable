package packet

import (
	"fmt"

	"sutext.github.io/cable/packet/coder"
)

type CloseCode uint8

const (
	CloseNormal                CloseCode = 0
	CloseKickedOut             CloseCode = 1
	CloseInvalidPacket         CloseCode = 2
	CloseInternalError         CloseCode = 3
	CloseDuplicateLogin        CloseCode = 4
	CloseAuthenticationFailure CloseCode = 5
	CloseAuthenticationTimeout CloseCode = 6
)

func (c CloseCode) String() string {
	switch c {
	case CloseNormal:
		return "Normal"
	case CloseKickedOut:
		return "Kicked Out"
	case CloseInvalidPacket:
		return "Invalid Packet"
	case CloseInternalError:
		return "Internal Error"
	case CloseDuplicateLogin:
		return "Duplicate Login"
	case CloseAuthenticationFailure:
		return "Authentication Failure"
	case CloseAuthenticationTimeout:
		return "Authentication Timeout"
	default:
		return "Unknown"
	}
}
func (c CloseCode) Error() string {
	return c.String()
}

type ClosePacket struct {
	Code CloseCode
}

func NewClose(code CloseCode) *ClosePacket {
	return &ClosePacket{Code: code}
}
func (p *ClosePacket) String() string {
	return fmt.Sprintf("CLOSE(%d)", p.Code)
}
func (p *ClosePacket) Type() PacketType {
	return CLOSE
}
func (p *ClosePacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CLOSE {
		return false
	}
	otherClose := other.(*ClosePacket)
	return p.Code == otherClose.Code
}

func (p *ClosePacket) EncodeTo(w coder.Writer) error {
	w.WriteUInt8(uint8(p.Code))
	return nil
}
func (p *ClosePacket) DecodeFrom(r coder.Reader) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = CloseCode(code)
	return nil
}
