package server

import (
	"fmt"
	"log/slog"

	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type ClosedHandler func(p *packet.Identity)
type ConnectHandler func(p *packet.Connect) packet.ConnectCode
type MessageHandler func(p *packet.Message, id *packet.Identity) error
type RequestHandler func(p *packet.Request, id *packet.Identity) (*packet.Response, error)

type Error uint8

const (
	ErrRequestTimeout Error = iota
	ErrServerIsClosed
	ErrConnectionClosed
	ErrSendingQueueFull
	ErrNetworkNotSupport
	ErrConnectionNotFound
)

func (e Error) Error() string {
	switch e {
	case ErrRequestTimeout:
		return "Request Timeout"
	case ErrServerIsClosed:
		return "Server Is Closed"
	case ErrConnectionClosed:
		return "Connection Closed"
	case ErrSendingQueueFull:
		return "Sending Queue Full"
	case ErrNetworkNotSupport:
		return "Network Not Support"
	case ErrConnectionNotFound:
		return "Connection Not Found"
	default:
		return "Unknown Error"
	}
}

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
	f func(*Options)
}
type Options struct {
	UseNIO  bool
	Logger  logger.Logger
	Network Network
	// QuicConfig     *quic.Config
	CloseHandler   ClosedHandler
	ConnectHandler ConnectHandler
	MessageHandler MessageHandler
	RequestHandler RequestHandler
}

func NewOptions(opts ...Option) *Options {
	var options = &Options{
		Network: NetworkTCP,
		UseNIO:  false,
		Logger:  logger.NewText(slog.LevelDebug),
		CloseHandler: func(p *packet.Identity) {

		},
		ConnectHandler: func(p *packet.Connect) packet.ConnectCode {
			return packet.ConnectAccepted
		},
		MessageHandler: func(p *packet.Message, id *packet.Identity) error {
			return fmt.Errorf("MessageHandler not implemented")
		},
		RequestHandler: func(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
			return nil, fmt.Errorf("RequestHandler not implemented")
		},
	}
	for _, o := range opts {
		o.f(options)
	}
	return options
}
func WithTCP() Option {
	return Option{f: func(o *Options) { o.Network = NetworkTCP }}
}
func WithNIO(useNIO bool) Option {
	return Option{f: func(o *Options) { o.UseNIO = useNIO }}
}
func WithUDP() Option {
	return Option{f: func(o *Options) { o.Network = NetworkUDP }}
}

//	func WithQUIC(config *quic.Config) Option {
//		return Option{f: func(o *Options) {
//			o.Network = NetworkQUIC
//			o.QuicConfig = config
//		}}
//	}
func WithLogger(logger logger.Logger) Option {
	return Option{f: func(o *Options) { o.Logger = logger }}
}
func WithClose(handler ClosedHandler) Option {
	return Option{f: func(o *Options) { o.CloseHandler = handler }}
}
func WithConnect(handler ConnectHandler) Option {
	return Option{f: func(o *Options) { o.ConnectHandler = handler }}
}
func WithMessage(handler MessageHandler) Option {
	return Option{f: func(o *Options) { o.MessageHandler = handler }}
}
func WithRequest(handler RequestHandler) Option {
	return Option{f: func(o *Options) { o.RequestHandler = handler }}
}
