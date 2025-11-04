package server

import (
	"context"
	"errors"

	"sutext.github.io/cable/packet"
)

type Conn interface {
	GetID() *packet.Identity
	Close(reason packet.CloseCode)
	SendPacket(p packet.Packet) error
}
type DataHandler func(id *packet.Identity, p *packet.DataPacket) (*packet.DataPacket, error)
type ConnHandler func(p *packet.ConnectPacket) packet.ConnectCode

type Server interface {
	Serve() error
	GetConn(cid string) (Conn, error)
	KickConn(cid string) error
	Shutdown(ctx context.Context) error
	HandleConn(handler ConnHandler)
	HandleData(handler DataHandler)
}

var (
	ErrConnectionClosed  = errors.New("connection_closed")
	ErrNetworkNotSupport = errors.New("network_not_supported")
	ErrConnctionNotFound = errors.New("connection_not_found")
)
