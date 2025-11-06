package packet

import (
	"fmt"

	"sutext.github.io/cable/packet/coder"
)

type Identity struct {
	UserID     string
	ClientID   string
	Credential string
}
type ConnectPacket struct {
	Version  uint8
	Identity *Identity
}

func NewConnect(identity *Identity) *ConnectPacket {
	return &ConnectPacket{Identity: identity, Version: 1}
}
func (p *ConnectPacket) String() string {
	return fmt.Sprintf("CONNECT(uid=%s, cid=%s)", p.Identity.UserID, p.Identity.ClientID)
}

func (p *ConnectPacket) Type() PacketType {
	return CONNECT
}
func (p *ConnectPacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNECT {
		return false
	}
	otherP := other.(*ConnectPacket)
	return p.Version == otherP.Version && p.Identity.Credential == otherP.Identity.Credential &&
		p.Identity.UserID == otherP.Identity.UserID &&
		p.Identity.ClientID == otherP.Identity.ClientID
}
func (p *ConnectPacket) EncodeTo(w coder.Writer) error {
	w.WriteUInt8(p.Version)
	if p.Identity != nil {
		w.WriteString(p.Identity.Credential)
		w.WriteString(p.Identity.UserID)
		w.WriteString(p.Identity.ClientID)
	}
	return nil
}

func (p *ConnectPacket) DecodeFrom(r coder.Reader) error {
	if version, err := r.ReadUInt8(); err != nil {
		return nil
	} else {
		p.Version = version
	}
	token, err := r.ReadString()
	if err != nil {
		return err
	}
	userID, err := r.ReadString()
	if err != nil {
		return err
	}
	clientID, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Identity = &Identity{
		Credential: token,
		UserID:     userID,
		ClientID:   clientID,
	}
	return nil
}

type ConnackCode uint8

const (
	ConnectionAccepted ConnackCode = 0
	ConnectionRejected ConnackCode = 1
)

type ConnackPacket struct {
	Code ConnackCode
}

func NewConnack(code ConnackCode) *ConnackPacket {
	return &ConnackPacket{
		Code: code,
	}
}
func (p *ConnackPacket) String() string {
	return fmt.Sprintf("CONNACK(%d)", p.Code)
}
func (P *ConnackPacket) Type() PacketType {
	return CONNACK
}
func (p *ConnackPacket) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNACK {
		return false
	}
	otherP := other.(*ConnackPacket)
	return p.Code == otherP.Code
}

func (p *ConnackPacket) EncodeTo(w coder.Writer) error {
	w.WriteUInt8(uint8(p.Code))
	return nil
}
func (p *ConnackPacket) DecodeFrom(r coder.Reader) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = ConnackCode(code)
	return nil
}
