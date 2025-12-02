package client

import (
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/packet"
)

type Network uint8

const (
	NewworkTCP Network = iota
	NetworkUDP
)

func (n Network) String() string {
	switch n {
	case NewworkTCP:
		return "tcp"
	case NetworkUDP:
		return "udp"
	default:
		return "unknown"
	}
}

type Handler interface {
	OnStatus(status Status)
	OnMessage(p *packet.Message) error
	OnRequest(p *packet.Request) (*packet.Response, error)
}
type emptyHandler struct{}

func (h *emptyHandler) OnStatus(status Status) {}

func (h *emptyHandler) OnMessage(p *packet.Message) error {
	return nil
}
func (h *emptyHandler) OnRequest(p *packet.Request) (*packet.Response, error) {
	return nil, nil
}

type Options struct {
	network        Network
	handler        Handler
	retrier        *Retrier
	pingTimeout    time.Duration
	pingInterval   time.Duration
	requestTimeout time.Duration
}

type Option struct {
	f func(*Options)
}

func newOptions(options ...Option) *Options {
	opts := &Options{
		handler:        &emptyHandler{},
		retrier:        NewRetrier(10, backoff.Exponential(2, 1.5)),
		network:        NewworkTCP,
		pingTimeout:    time.Second * 5,
		pingInterval:   time.Second * 60,
		requestTimeout: time.Second * 5,
	}
	for _, o := range options {
		o.f(opts)
	}
	return opts
}
func WithNetwork(network Network) Option {
	return Option{f: func(o *Options) {
		o.network = network
	}}
}
func WithRetrier(retrier *Retrier) Option {
	return Option{f: func(o *Options) {
		o.retrier = retrier
	}}
}
func WithKeepAlive(timeout, interval time.Duration) Option {
	return Option{f: func(o *Options) {
		o.pingTimeout = timeout
		o.pingInterval = interval
	}}
}
func WithHandler(handler Handler) Option {
	return Option{f: func(o *Options) {
		o.handler = handler
	}}
}
func WithRequestTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.requestTimeout = timeout
	}}
}
