package broker

import (
	"log/slog"

	"sutext.github.io/cable/internal/logger"
)

type Listener struct {
	Address string
	Network string
}

type options struct {
	peers       []string
	logger      logger.Logger
	brokerID    string
	listeners   []Listener
	queueWorker int
	queueBuffer int
}

func newOptions(opts ...Option) *options {
	options := &options{
		logger:      logger.NewJSON(slog.LevelDebug),
		queueWorker: 32,
		queueBuffer: 100,
	}
	for _, opt := range opts {
		opt.f(options)
	}
	return options
}

type Option struct {
	f func(*options)
}

// WithPeers sets the list of peers to connect to.
// The list should contain the addresses of the other brokers in the cluster.
// eg []string{"broker1@127.0.0.1:8080", "broker2@127.0.0.1:8081"}
func WithPeers(peers []string) Option {
	return Option{func(o *options) {
		o.peers = peers
	}}
}
func WithLogger(l logger.Logger) Option {
	return Option{func(o *options) {
		o.logger = l
	}}
}

// WithQueue sets the number of worker and buffer for the queue.
func WithQueue(worker, buffer int) Option {
	return Option{func(o *options) {
		o.queueWorker = worker
		o.queueBuffer = buffer
	}}
}

// WithListener sets the listener for the broker.
func WithListeners(ls ...Listener) Option {
	return Option{func(o *options) {
		o.listeners = ls
	}}
}

func WithBrokerID(id string) Option {
	return Option{func(o *options) {
		o.brokerID = id
	}}
}
