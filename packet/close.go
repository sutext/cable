package packet

import (
	"fmt"

	"sutext.github.io/cable/coder"
)

type CloseCode uint8

const (
	CloseNormal                CloseCode = 0
	CloseKickedOut             CloseCode = 1
	CloseNoHeartbeat           CloseCode = 2
	CloseInvalidPacket         CloseCode = 3
	ClosePintTimeOut           CloseCode = 4
	CloseInternalError         CloseCode = 5
	CloseDuplicateLogin        CloseCode = 6
	CloseAuthenticationFailure CloseCode = 7
	CloseAuthenticationTimeout CloseCode = 8
)

var closeCodeMap = map[CloseCode]string{
	CloseNormal:                "Normal",
	CloseKickedOut:             "Kicked Out",
	CloseNoHeartbeat:           "No Heartbeat",
	ClosePintTimeOut:           "Pint Time Out",
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

func (p *Close) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return nil
}
func (p *Close) ReadFrom(r coder.Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = CloseCode(code)
	return nil
}
