package server

import (
	"context"

	"sutext.github.io/cable/packet"
)

type Conn interface {
	ID() *packet.Identity
	Close(code packet.CloseCode)
	IsActive() bool
	Request(ctx context.Context, p *packet.Request) (*packet.Response, error)
	SendMessage(p *packet.Message) error
}
type ConnectHandler func(p *packet.Connect) packet.ConnackCode
type MessageHandler func(p *packet.Message, id *packet.Identity) error
type RequestHandler func(p *packet.Request, id *packet.Identity) (*packet.Response, error)

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
