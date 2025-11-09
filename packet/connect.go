package packet

import (
	"fmt"
)

type Identity struct {
	UserID     string
	ClientID   string
	Credential string
}
type Connect struct {
	Version  uint8
	Identity *Identity
}

func NewConnect(identity *Identity) *Connect {
	return &Connect{Identity: identity, Version: 1}
}
func (p *Connect) String() string {
	return fmt.Sprintf("CONNECT(uid=%s, cid=%s)", p.Identity.UserID, p.Identity.ClientID)
}

func (p *Connect) Type() PacketType {
	return CONNECT
}
func (p *Connect) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNECT {
		return false
	}
	otherP := other.(*Connect)
	return p.Version == otherP.Version && p.Identity.Credential == otherP.Identity.Credential &&
		p.Identity.UserID == otherP.Identity.UserID &&
		p.Identity.ClientID == otherP.Identity.ClientID
}
func (p *Connect) EncodeTo(w Encoder) error {
	w.WriteUInt8(p.Version)
	if p.Identity != nil {
		w.WriteString(p.Identity.Credential)
		w.WriteString(p.Identity.UserID)
		w.WriteString(p.Identity.ClientID)
	}
	return nil
}

func (p *Connect) DecodeFrom(r Decoder) error {
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

func (c ConnackCode) String() string {
	switch c {
	case ConnectionAccepted:
		return "Connection Accepted"
	case ConnectionRejected:
		return "Connection Rejected"
	default:
		return "Unknown Connack Code"
	}
}

type Connack struct {
	Code ConnackCode
}

func NewConnack(code ConnackCode) *Connack {
	return &Connack{
		Code: code,
	}
}
func (p *Connack) String() string {
	return fmt.Sprintf("CONNACK:%s", p.Code.String())
}
func (P *Connack) Type() PacketType {
	return CONNACK
}
func (p *Connack) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNACK {
		return false
	}
	otherP := other.(*Connack)
	return p.Code == otherP.Code
}

func (p *Connack) EncodeTo(w Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return nil
}
func (p *Connack) DecodeFrom(r Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = ConnackCode(code)
	return nil
}
