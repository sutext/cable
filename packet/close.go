package packet

import (
	"fmt"

	"sutext.github.io/cable/buffer"
)

type CloseCode uint16

const (
	CloseNormal                CloseCode = 1000
	CloseGoingAway             CloseCode = 1001
	CloseAlreayLogined         CloseCode = 1002
	CloseUnsupported           CloseCode = 1003
	CloseNoStatus              CloseCode = 1004
	CloseInvalidFrame          CloseCode = 1005
	CloseMessageTooBig         CloseCode = 1006
	CloseInternalError         CloseCode = 1007
	CloseServiceRestart        CloseCode = 1008
	CloseDuplicateLogin        CloseCode = 1009
	CloseAuthenticationFailure CloseCode = 1010
	CloseAuthenticationTimeout CloseCode = 1011
	CloseKickedOut             CloseCode = 1012
)

func (c CloseCode) String() string {
	switch c {
	case CloseNormal:
		return "Normal"
	case CloseGoingAway:
		return "Going Away"
	case CloseAlreayLogined:
		return "Already Logined"
	case CloseUnsupported:
		return "Unsupported"
	case CloseNoStatus:
		return "No Status"
	case CloseInvalidFrame:
		return "Invalid Frame"
	case CloseMessageTooBig:
		return "Message Too Big"
	case CloseInternalError:
		return "Internal Error"
	case CloseServiceRestart:
		return "Service Restart"
	case CloseDuplicateLogin:
		return "Duplicate Login"
	case CloseAuthenticationFailure:
		return "Authentication Failure"
	case CloseAuthenticationTimeout:
		return "Authentication Timeout"
	case CloseKickedOut:
		return "Kicked Out"
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
func (p *ClosePacket) WriteTo(w *buffer.Buffer) error {
	w.WriteUInt16(uint16(p.Code))
	return nil
}

func (p *ClosePacket) ReadFrom(buffer *buffer.Buffer) error {
	code, err := buffer.ReadUInt16()
	if err != nil {
		return err
	}
	p.Code = CloseCode(code)
	return nil
}
