package server

import (
	"fmt"

	"sutext.github.io/cable/packet"
)

type ClosedHandler func(p *packet.Identity)
type ConnectHandler func(p *packet.Connect) packet.ConnectCode
type MessageHandler func(p *packet.Message, id *packet.Identity) error
type RequestHandler func(p *packet.Request, id *packet.Identity) (*packet.Response, error)

type Network uint8

const (
	NetworkTCP Network = iota
	NetworkUDP
	NetworkQUIC
	NetworkWebSocket
)

func (n Network) String() string {
	switch n {
	case NetworkTCP:
		return "TCP"
	case NetworkUDP:
		return "UDP"
	case NetworkQUIC:
		return "QUIC"
	case NetworkWebSocket:
		return "WebSocket"
	default:
		return "Unknown"
	}
}

type Option struct {
	f func(*options)
}
type options struct {
	useNIO  bool
	network Network
	// QuicConfig     *quic.Config
	closeHandler   ClosedHandler
	connectHandler ConnectHandler
	messageHandler MessageHandler
	requestHandler RequestHandler
}

func NewOptions(opts ...Option) *options {
	var options = &options{
		useNIO:  false,
		network: NetworkTCP,
		closeHandler: func(p *packet.Identity) {

		},
		connectHandler: func(p *packet.Connect) packet.ConnectCode {
			return packet.ConnectAccepted
		},
		messageHandler: func(p *packet.Message, id *packet.Identity) error {
			return fmt.Errorf("MessageHandler not implemented")
		},
		requestHandler: func(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
			return nil, fmt.Errorf("RequestHandler not implemented")
		},
	}
	for _, o := range opts {
		o.f(options)
	}
	return options
}
func WithTCP() Option {
	return Option{f: func(o *options) { o.network = NetworkTCP }}
}
func WithNIO(useNIO bool) Option {
	return Option{f: func(o *options) { o.useNIO = useNIO }}
}
func WithUDP() Option {
	return Option{f: func(o *options) { o.network = NetworkUDP }}
}

//	func WithQUIC(config *quic.Config) Option {
//		return Option{f: func(o *Options) {
//			o.Network = NetworkQUIC
//			o.QuicConfig = config
//		}}
//	}

func WithClose(handler ClosedHandler) Option {
	return Option{f: func(o *options) { o.closeHandler = handler }}
}
func WithConnect(handler ConnectHandler) Option {
	return Option{f: func(o *options) { o.connectHandler = handler }}
}
func WithMessage(handler MessageHandler) Option {
	return Option{f: func(o *options) { o.messageHandler = handler }}
}
func WithRequest(handler RequestHandler) Option {
	return Option{f: func(o *options) { o.requestHandler = handler }}
}
