package server

import (
	"log/slog"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Network uint8

const (
	NetworkWS Network = iota
	NetworkTCP
	NetworkQUIC
)

func (n Network) String() string {
	switch n {
	case NetworkWS:
		return "WebSocket"
	case NetworkTCP:
		return "TCP"
	case NetworkQUIC:
		return "QUIC"
	default:
		return "Unknown"
	}
}

type Option struct {
	f func(*Options)
}
type Options struct {
	UseNIO         bool
	Logger         logger.Logger
	Network        Network
	QuicConfig     *quic.Config
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
		CloseHandler: func(s Server, p *packet.Identity) {

		},
		ConnectHandler: func(s Server, p *packet.Connect) packet.ConnackCode {
			return packet.ConnectionAccepted
		},
		MessageHandler: func(s Server, p *packet.Message, id *packet.Identity) {

		},
		RequestHandler: func(s Server, p *packet.Request, id *packet.Identity) (*packet.Response, error) {
			return nil, nil
		},
	}
	for _, o := range opts {
		o.f(options)
	}
	return options
}
func WithQUIC(config *quic.Config) Option {
	return Option{f: func(o *Options) {
		o.Network = NetworkQUIC
		o.QuicConfig = config
	}}
}
func WithTCP() Option {
	return Option{f: func(o *Options) { o.Network = NetworkTCP }}
}
func WithNIO(useNIO bool) Option {
	return Option{f: func(o *Options) { o.UseNIO = useNIO }}
}
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
