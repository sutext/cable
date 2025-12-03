package packet

import (
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type CloseCode uint8

const (
	CloseNormal                CloseCode = 0
	CloseKickedOut             CloseCode = 1
	CloseNoHeartbeat           CloseCode = 2
	CloseInvalidPacket         CloseCode = 3
	ClosePingTimeOut           CloseCode = 4
	CloseInternalError         CloseCode = 5
	CloseDuplicateLogin        CloseCode = 6
	CloseAuthenticationFailure CloseCode = 7
	CloseAuthenticationTimeout CloseCode = 8
)

var closeCodeMap = map[CloseCode]string{
	CloseNormal:                "Normal",
	CloseKickedOut:             "Kicked Out",
	CloseNoHeartbeat:           "No Heartbeat",
	ClosePingTimeOut:           "Ping Time Out",
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
func AsCloseCode(err error) CloseCode {
	switch e := err.(type) {
	case CloseCode:
		return e
	case Error, coder.Error:
		return CloseInvalidPacket
	default:
		return CloseInternalError
	}
}

type Close struct {
	packet
	Code CloseCode
}

func NewClose(code CloseCode) *Close {
	return &Close{
		Code: code,
	}
}
func (p *Close) Type() PacketType {
	return CLOSE
}
func (p *Close) String() string {
	return fmt.Sprintf("CLOSE(Code=%s, Props=%v)", p.Code, p.props)
}

func (p *Close) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CLOSE {
		return false
	}
	otherClose := other.(*Close)
	return maps.Equal(p.props, otherClose.props) && p.Code == otherClose.Code
}

func (p *Close) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return p.packet.WriteTo(w)
}
func (p *Close) ReadFrom(r coder.Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = CloseCode(code)
	return p.packet.ReadFrom(r)
}
