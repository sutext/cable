package client

import (
	"runtime"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Network string

const (
	NewworkTCP Network = "tcp"
	NetworkUDP Network = "udp"
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
	logger              *xlog.Logger
	network             Network
	handler             Handler
	retrier             *Retrier
	pingTimeout         time.Duration
	pingInterval        time.Duration
	requestTimeout      time.Duration
	sendQueueCapacity   int
	recvPollCapacity    int
	recvPollWorkerCount int
}

type Option struct {
	f func(*Options)
}

func newOptions(options ...Option) *Options {
	opts := &Options{
		logger:              xlog.With("GROUP", "CLIENT"),
		handler:             &emptyHandler{},
		retrier:             NewRetrier(10, backoff.Exponential(time.Second, 1.5)),
		network:             NewworkTCP,
		pingTimeout:         time.Second * 5,
		pingInterval:        time.Second * 60,
		requestTimeout:      time.Second * 5,
		sendQueueCapacity:   1024,
		recvPollCapacity:    1024,
		recvPollWorkerCount: runtime.NumCPU(),
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
func WithRequest(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.requestTimeout = timeout
	}}
}
func WithSendQueue(capacity int) Option {
	return Option{f: func(o *Options) {
		o.sendQueueCapacity = capacity
	}}
}
func WithRecvPoll(capacity, workerCount int) Option {
	return Option{f: func(o *Options) {
		o.recvPollCapacity = capacity
		o.recvPollWorkerCount = workerCount
	}}
}
