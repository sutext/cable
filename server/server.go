package server

import (
	"context"

	"sutext.github.io/cable/packet"
)

type Conn interface {
	ID() *packet.Identity
	Close(code packet.CloseCode)
	Request(ctx context.Context, p *packet.RequestPacket) (*packet.ResponsePacket, error)
	SendMessage(p *packet.MessagePacket) error
}

type ConnectHandler func(p *packet.ConnectPacket) packet.ConnackCode
type MessageHandler func(p *packet.MessagePacket, id *packet.Identity) error
type RequestHandler func(p *packet.RequestPacket, id *packet.Identity) (*packet.ResponsePacket, error)

type Server interface {
	Serve() error
	GetConn(cid string) (Conn, error)
	KickConn(cid string) error
	Shutdown(ctx context.Context) error
}
type Error uint8

const (
	ErrConnectionClosed  Error = 1
	ErrNetworkNotSupport Error = 2
	ErrConnctionNotFound Error = 3
)

func (e Error) Error() string {
	switch e {
	case ErrConnectionClosed:
		return "Connection Closed"
	case ErrNetworkNotSupport:
		return "Network Not Support"
	case ErrConnctionNotFound:
		return "Connection Not Found"
	default:
		return "Unknown Error"
	}
}
