package packet

import (
	"fmt"
	"maps"

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
func (i *Identity) String() string {
	return fmt.Sprintf("uid=%s, cid=%s", i.UserID, i.ClientID)
}

const (
	versionMask uint8 = 0x3f
)

type Connect struct {
	packet
	Version  uint8
	Identity *Identity
}

func NewConnect(identity *Identity) *Connect {
	return &Connect{
		Identity: identity,
		Version:  1,
	}
}
func (p *Connect) Type() PacketType {
	return CONNECT
}
func (p *Connect) String() string {
	return fmt.Sprintf("CONNECT(Version=%d, %s, Props=%v)", p.Version, p.Identity.String(), p.props)
}

func (p *Connect) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNECT {
		return false
	}
	o := other.(*Connect)
	return maps.Equal(p.props, o.props) && p.Version == o.Version && p.Identity.Equal(o.Identity)
}
func (p *Connect) WriteTo(w coder.Encoder) error {
	flags := uint8(0)
	flags |= p.Version & versionMask
	w.WriteUInt8(flags)
	err := p.Identity.WriteTo(w)
	if err != nil {
		return err
	}
	return p.packet.WriteTo(w)
}

func (p *Connect) ReadFrom(r coder.Decoder) error {
	flags, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	identity := &Identity{}
	err = identity.ReadFrom(r)
	if err != nil {
		return err
	}
	p.Version = flags & versionMask
	p.Identity = identity
	return p.packet.ReadFrom(r)
}

type ConnectCode uint8

const (
	ConnectAccepted  ConnectCode = 0
	ConnectRejected  ConnectCode = 1
	ConnectDuplicate ConnectCode = 2
)

func (c ConnectCode) String() string {
	switch c {
	case ConnectAccepted:
		return "Connection Accepted"
	case ConnectRejected:
		return "Connection Rejected"
	case ConnectDuplicate:
		return "Connection Duplicate"
	default:
		return "Unknown Connack Code"
	}
}
func (c ConnectCode) Error() string {
	return c.String()
}

type Connack struct {
	packet
	Code ConnectCode
}

func NewConnack(code ConnectCode) *Connack {
	return &Connack{
		Code: code,
	}
}
func (p *Connack) Type() PacketType {
	return CONNACK
}
func (p *Connack) Equal(other Packet) bool {
	if other == nil {
		return false
	}
	if other.Type() != CONNACK {
		return false
	}
	o := other.(*Connack)
	return maps.Equal(p.props, o.props) && p.Code == o.Code
}
func (p *Connack) String() string {
	return fmt.Sprintf("CONNACK(Code=%s,Props=%v)", p.Code, p.props)
}
func (p *Connack) WriteTo(w coder.Encoder) error {
	w.WriteUInt8(uint8(p.Code))
	return p.packet.WriteTo(w)
}
func (p *Connack) ReadFrom(r coder.Decoder) error {
	code, err := r.ReadUInt8()
	if err != nil {
		return err
	}
	p.Code = ConnectCode(code)
	return p.packet.ReadFrom(r)
}
