package packet

import (
	"fmt"
)

type CloseCode uint8

const (
	CloseNormal                CloseCode = 0
	CloseKickedOut             CloseCode = 1
	CloseNoHeartbeat           CloseCode = 2
	CloseInvalidPacket         CloseCode = 3
	CloseInternalError         CloseCode = 4
	CloseDuplicateLogin        CloseCode = 5
	CloseAuthenticationFailure CloseCode = 6
	CloseAuthenticationTimeout CloseCode = 7
)

var closeCodeMap = map[CloseCode]string{
	CloseNormal:                "Normal",
	CloseKickedOut:             "Kicked Out",
	CloseNoHeartbeat:           "No Heartbeat",
	CloseInvalidPacket:         "Invalid Packet",
	CloseInternalError:         "Internal Error",
	CloseDuplicateLogin:        "Duplicate Login",
	CloseAuthenticationFailure: "Authentication Failure",
	CloseAuthenticationTimeout: "Authentication Timeout",
}

func (c CloseCode) String() string {
	if s, ok := closeCodeMap[c]; ok {
		return s
	}
	return "Unknown"
}
func (c CloseCode) Error() string {
	return c.String()
}

type Close struct {
	Code CloseCode
}

func NewClose(code CloseCode) *Close {
	return &Close{Code: code}
}
func (p *Close) String() string {
	return fmt.Sprintf("CLOSE(%d)", p.Code)
}
func (p *Close) Type() PacketType {
	return CLOSE
}
func (p *Close) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CLOSE {
		return false
	}
	otherClose := other.(*Close)
	return p.Code == otherClose.Code
}

func (p *Close) EncodeTo(w Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return nil
}
func (p *Close) DecodeFrom(r Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = CloseCode(code)
	return nil
}
