package packet

import (
	"fmt"

	"sutext.github.io/cable/coder"
)

type Identity struct {
	UserID   string
	ClientID string
	Password string
}

func (i *Identity) WriteTo(w coder.Encoder) error {
	w.WriteString(i.UserID)
	w.WriteString(i.ClientID)
	w.WriteString(i.Password)
	return nil
}

func (i *Identity) ReadFrom(r coder.Decoder) error {
	userID, err := r.ReadString()
	if err != nil {
		return err
	}
	clientID, err := r.ReadString()
	if err != nil {
		return err
	}
	password, err := r.ReadString()
	if err != nil {
		return err
	}
	i.UserID = userID
	i.ClientID = clientID
	i.Password = password
	return nil
}
func (i *Identity) Equal(other *Identity) bool {
	if other == nil {
		return false
	}
	return i.UserID == other.UserID && i.ClientID == other.ClientID && i.Password == other.Password
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
	return p.Version == otherP.Version && p.Identity.Equal(otherP.Identity)
}
func (p *Connect) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(p.Version)
	return p.Identity.WriteTo(w)
}

func (p *Connect) ReadFrom(r coder.Decoder) error {
	version, err := r.ReadUInt8()
	if err != nil {
		return nil
	}
	var identity Identity
	err = identity.ReadFrom(r)
	if err != nil {
		return err
	}
	p.Version = version
	p.Identity = &identity
	return nil
}

type ConnackCode uint8

const (
	ConnectionAccepted  ConnackCode = 0
	ConnectionRejected  ConnackCode = 1
	ConnectionDuplicate ConnackCode = 2
)

func (c ConnackCode) String() string {
	switch c {
	case ConnectionAccepted:
		return "Connection Accepted"
	case ConnectionRejected:
		return "Connection Rejected"
	case ConnectionDuplicate:
		return "Connection Duplicate"
	default:
		return "Unknown Connack Code"
	}
}
func (c ConnackCode) Error() string {
	return c.String()
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

func (p *Connack) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return nil
}
func (p *Connack) ReadFrom(r coder.Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = ConnackCode(code)
	return nil
}
