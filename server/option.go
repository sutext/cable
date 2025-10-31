package server

import (
	qc "golang.org/x/net/quic"
)

type Option struct {
	F func(*Options)
}
type Options struct {
	Network    string
	Address    string
	UseNIO     bool
	QuicConfig *qc.Config
}

func (o *Options) Apply(opts ...Option) {
	for _, opt := range opts {
		opt.F(o)
	}
}
func NewOptions(opts ...Option) *Options {
	var options = &Options{}
	for _, o := range opts {
		o.F(options)
	}
	return options
}
func WithQUIC(address string, config *qc.Config) Option {
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
