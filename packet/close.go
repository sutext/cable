package packet

import (
	"fmt"
	"maps"

	"sutext.github.io/cable/coder"
)

type CloseCode uint8

const (
	CloseNormal CloseCode = iota
	CloseKickedOut
	CloseNoHeartbeat
	ClosePingTimeOut
	CloseAuthFailure
	CloseAuthTimeout
	CloseInvalidPacket
	CloseInternalError
	CloseDuplicateLogin
)

var closeCodeMap = map[CloseCode]string{
	CloseNormal:         "Normal",
	CloseKickedOut:      "Kicked Out",
	CloseNoHeartbeat:    "No Heartbeat",
	ClosePingTimeOut:    "Ping Time Out",
	CloseAuthFailure:    "Authentication Failure",
	CloseAuthTimeout:    "Authentication Timeout",
	CloseInvalidPacket:  "Invalid Packet",
	CloseInternalError:  "Internal Error",
	CloseDuplicateLogin: "Duplicate Login",
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
func (p *Close) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CLOSE {
		return false
	}
	o := other.(*Close)
	return maps.Equal(p.props, o.props) && p.Code == o.Code
}
func (p *Close) String() string {
	return fmt.Sprintf("CLOSE(Code=%s, Props=%v)", p.Code, p.props)
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
