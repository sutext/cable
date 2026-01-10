package client

import (
	"time"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

const (
	NetworkTCP       string = "tcp"
	NetworkUDP       string = "udp"
	NetworkQUIC      string = "quic"
	NetworkWebSocket string = "ws"
)

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
	logger            *xlog.Logger
	handler           Handler
	retrier           *Retrier
	network           string
	quicConfig        *quic.Config
	pingTimeout       time.Duration
	pingInterval      time.Duration
	writeTimeout      time.Duration
	requestTimeout    time.Duration
	messageTimeout    time.Duration
	sendQueueCapacity int32
}

type Option struct {
	f func(*Options)
}

func newOptions(options ...Option) *Options {
	opts := &Options{
		logger:            xlog.With("GROUP", "CLIENT"),
		handler:           &emptyHandler{},
		retrier:           NewRetrier(10, backoff.Exponential(time.Second, 1.5)),
		network:           NetworkTCP,
		pingTimeout:       time.Second * 5,
		pingInterval:      time.Second * 60,
		writeTimeout:      time.Second * 5,
		requestTimeout:    time.Second * 5,
		messageTimeout:    time.Second * 5,
		sendQueueCapacity: 1024,
	}
	for _, o := range options {
		o.f(opts)
	}
	return opts
}
func WithPing(timeout, interval time.Duration) Option {
	return Option{f: func(o *Options) {
		o.pingTimeout = timeout
		o.pingInterval = interval
	}}
}
func WithLogger(logger *xlog.Logger) Option {
	return Option{f: func(o *Options) {
		o.logger = logger
	}}
}
func WithNetwork(network string) Option {
	return Option{f: func(o *Options) {
		o.network = network
	}}
}
func WithRetrier(retrier *Retrier) Option {
	return Option{f: func(o *Options) {
		o.retrier = retrier
	}}
}
func WithQuicConfig(config *quic.Config) Option {
	return Option{f: func(o *Options) {
		o.quicConfig = config
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
func WithWriteTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.writeTimeout = timeout
	}}
}
func WithRequestTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.requestTimeout = timeout
	}}
}
func WithMessageTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.messageTimeout = timeout
	}}
}
func WithSendQueue(capacity int32) Option {
	return Option{f: func(o *Options) {
		o.sendQueueCapacity = capacity
	}}
}
