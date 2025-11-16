package packet

import (
	"fmt"

	"sutext.github.io/cable/coder"
)

type CloseCode uint8

const (
	CloseNormal                CloseCode = 1
	CloseKickedOut             CloseCode = 2
	CloseNoHeartbeat           CloseCode = 3
	CloseInvalidPacket         CloseCode = 4
	ClosePintTimeOut           CloseCode = 5
	CloseInternalError         CloseCode = 6
	CloseDuplicateLogin        CloseCode = 7
	CloseAuthenticationFailure CloseCode = 8
	CloseAuthenticationTimeout CloseCode = 9
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
