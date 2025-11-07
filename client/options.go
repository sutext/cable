package client

import (
	"log/slog"
	"math"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Options struct {
	logger         logger.Logger
	address        string
	retryLimit     int
	retryBackoff   backoff.Backoff
	pingTimeout    time.Duration
	pingInterval   time.Duration
	statusHandler  StatusHandler
	messageHandler MessageHandler
	requestHandler RequestHandler
	requestTimeout time.Duration
}

type Option struct {
	f func(*Options)
}

func newOptions(options ...Option) *Options {
	opts := &Options{
		logger:         logger.NewJSON(slog.LevelDebug),
		retryLimit:     math.MaxInt,
		retryBackoff:   backoff.Default(),
		pingTimeout:    time.Second * 5,
		pingInterval:   time.Second * 60,
		statusHandler:  func(status Status) {},
		messageHandler: func(p *packet.MessagePacket) error { return nil },
		requestHandler: func(p *packet.RequestPacket) (*packet.ResponsePacket, error) { return nil, nil },
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
func WithStatusHandler(handler StatusHandler) Option {
	return Option{f: func(o *Options) {
		o.statusHandler = handler
	}}
}
func WithMessageHandler(handler MessageHandler) Option {
	return Option{f: func(o *Options) {
		o.messageHandler = handler
	}}
}
func WithRequestHandler(handler RequestHandler) Option {
	return Option{f: func(o *Options) {
		o.requestHandler = handler
	}}
}
func WithRequestTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.requestTimeout = timeout
	}}
}
