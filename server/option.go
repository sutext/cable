package server

import (
	"log/slog"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/logger"
)

type Option struct {
	F func(*Options)
}
type Options struct {
	Network    string
	Address    string
	UseNIO     bool
	Logger     logger.Logger
	QuicConfig *quic.Config
}

func NewOptions(opts ...Option) *Options {
	var options = &Options{
		Logger: logger.NewJSON(slog.LevelInfo),
	}
	for _, o := range opts {
		o.F(options)
	}
	return options
}
func WithQUIC(address string, config *quic.Config) Option {
	return Option{F: func(o *Options) {
		o.Network = "quic"
		o.Address = address
		o.QuicConfig = config
	}}
}
func WithTCP(address string) Option {
	return Option{F: func(o *Options) { o.Network = "tcp"; o.Address = address }}
}
func WithNIO(useNIO bool) Option {
	return Option{F: func(o *Options) { o.UseNIO = useNIO }}
}
func WithLogger(logger logger.Logger) Option {
	return Option{F: func(o *Options) { o.Logger = logger }}
}
