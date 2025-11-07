package client

import (
	"log/slog"
	"math"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Handler interface {
	OnStatus(status Status)
	OnMessage(p *packet.Message)
	OnRequest(p *packet.Request) (*packet.Response, error)
}
type emptyHandler struct{}

func (h *emptyHandler) OnStatus(status Status)      {}
func (h *emptyHandler) OnMessage(p *packet.Message) {}
func (h *emptyHandler) OnRequest(p *packet.Request) (*packet.Response, error) {
	return nil, nil
}

type Options struct {
	logger         logger.Logger
	address        string
	handler        Handler
	retryLimit     int
	pingTimeout    time.Duration
	pingInterval   time.Duration
	retryBackoff   backoff.Backoff
	requestTimeout time.Duration
}

type Option struct {
	f func(*Options)
}

func newOptions(options ...Option) *Options {
	opts := &Options{
		logger:       logger.NewJSON(slog.LevelDebug),
		handler:      &emptyHandler{},
		retryLimit:   math.MaxInt,
		retryBackoff: backoff.Default(),
		pingTimeout:  time.Second * 5,
		pingInterval: time.Second * 60,
	}
	for _, o := range options {
		o.f(opts)
	}
	return opts
}
func WithAddress(address string) Option {
	return Option{f: func(o *Options) {
		o.address = address
	}}
}
func WithLogger(logger logger.Logger) Option {
	return Option{f: func(o *Options) {
		o.logger = logger
	}}
}
func WithRetry(limit int, backoff backoff.Backoff) Option {
	return Option{f: func(o *Options) {
		o.retryLimit = limit
		o.retryBackoff = backoff
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
