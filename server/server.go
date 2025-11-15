package server

import (
	"context"

	"sutext.github.io/cable/packet"
)

type Conn interface {
	ID() *packet.Identity
	Close(code packet.CloseCode)
	IsActive() bool
	SendMessage(p *packet.Message) error
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
}
type ClosedHandler func(s Server, p *packet.Identity)
type ConnectHandler func(s Server, p *packet.Connect) packet.ConnackCode
type MessageHandler func(s Server, p *packet.Message, id *packet.Identity)
type RequestHandler func(s Server, p *packet.Request, id *packet.Identity) (*packet.Response, error)

type Server interface {
	Serve() error
	Network() Network
	GetConn(cid string) (Conn, bool)
	KickConn(cid string) bool
	Shutdown(ctx context.Context) error
}

type Error uint8

const (
	ErrRequestTimeout    Error = 0
	ErrConnectionClosed  Error = 1
	ErrSendingQueueFull  Error = 2
	ErrNetworkNotSupport Error = 3
	ErrConnctionNotFound Error = 4
)

func (e Error) Error() string {
	switch e {
	case ErrRequestTimeout:
		return "Request Timeout"
	case ErrConnectionClosed:
		return "Connection Closed"
	case ErrSendingQueueFull:
		return "Sending Queue Full"
	case ErrNetworkNotSupport:
		return "Network Not Support"
	case ErrConnctionNotFound:
		return "Connection Not Found"
	default:
		return "Unknown Error"
	}
}
