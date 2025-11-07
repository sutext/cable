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
	Network        string
	UseNIO         bool
	Logger         logger.Logger
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
		ConnectHandler: func(p *packet.ConnectPacket) packet.ConnackCode {
			return packet.ConnectionAccepted
		},
		MessageHandler: func(p *packet.MessagePacket, id *packet.Identity) error {
			return nil
		},
		RequestHandler: func(p *packet.RequestPacket, id *packet.Identity) (*packet.ResponsePacket, error) {
			return &packet.ResponsePacket{Serial: p.Serial, Body: p.Body}, nil
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
func WithConnectHandler(handler ConnectHandler) Option {
	return Option{f: func(o *Options) { o.ConnectHandler = handler }}
}
func WithMessageHandler(handler MessageHandler) Option {
	return Option{f: func(o *Options) { o.MessageHandler = handler }}
}
