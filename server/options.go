package server

import (
	"log/slog"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Option struct {
	f func(*Options)
}
type Options struct {
	UseNIO         bool
	Logger         logger.Logger
	Network        string
	QuicConfig     *quic.Config
	ConnectHandler ConnectHandler
	MessageHandler MessageHandler
	RequestHandler RequestHandler
}

func NewOptions(opts ...Option) *Options {
	var options = &Options{
		Network: "tcp",
		UseNIO:  true,
		Logger:  logger.NewJSON(slog.LevelInfo),
		ConnectHandler: func(p *packet.Connect) packet.ConnackCode {
			return packet.ConnectionAccepted
		},
		MessageHandler: func(p *packet.Message, id *packet.Identity) error {
			return nil
		},
		RequestHandler: func(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
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
		o.Network = "quic"
		o.QuicConfig = config
	}}
}
func WithTCP() Option {
	return Option{f: func(o *Options) { o.Network = "tcp" }}
}
func WithNIO(useNIO bool) Option {
	return Option{f: func(o *Options) { o.UseNIO = useNIO }}
}
func WithLogger(logger logger.Logger) Option {
	return Option{f: func(o *Options) { o.Logger = logger }}
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
