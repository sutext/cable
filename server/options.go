package server

import (
	"fmt"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type ClosedHandler func(p *packet.Identity)
type ConnectHandler func(p *packet.Connect) packet.ConnectCode
type MessageHandler func(p *packet.Message, id *packet.Identity) error
type RequestHandler func(p *packet.Request, id *packet.Identity) (*packet.Response, error)

const (
	NetworkTCP       string = "tcp"
	NetworkUDP       string = "udp"
	NetworkQUIC      string = "quic"
	NetworkGRPC      string = "grpc"
	NetworkWebSocket string = "websocket"
)

type Option struct {
	f func(*options)
}
type options struct {
	logger         *xlog.Logger
	network        string
	queueCapacity  int32
	closeHandler   ClosedHandler
	connectHandler ConnectHandler
	messageHandler MessageHandler
	requestHandler RequestHandler
}

func NewOptions(opts ...Option) *options {
	var options = &options{
		logger:        xlog.With("GROUP", "SERVER"),
		network:       NetworkTCP,
		queueCapacity: 1024,
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
func WithNetwork(network string) Option {
	return Option{f: func(o *options) { o.network = network }}
}

//	func WithQUIC(config *quic.Config) Option {
//		return Option{f: func(o *Options) {
//			o.Network = NetworkQUIC
//			o.QuicConfig = config
//		}}
//	}

func WithLogger(logger *xlog.Logger) Option {
	return Option{f: func(o *options) { o.logger = logger }}
}
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
func WithSendQueue(cap int32) Option {
	return Option{f: func(o *options) { o.queueCapacity = cap }}
}
