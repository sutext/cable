package cluster

import (
	"context"
	"sync/atomic"

	"sutext.github.io/cable/server"
)

type Listener struct {
	server.Server
	started   atomic.Bool
	network   string
	address   string
	options   []server.Option
	autoStart bool
}

func NewListener(network, address string, autoStart bool, opts ...server.Option) *Listener {
	return &Listener{
		network:   network,
		address:   address,
		options:   append(opts, server.WithNetwork(network)),
		autoStart: autoStart,
	}
}
func (l *Listener) AddOptions(opts ...server.Option) {
	l.options = append(l.options, opts...)
}
func (l *Listener) ISStarted() bool {
	return l.started.Load()
}
func (l *Listener) Start(opts ...server.Option) {
	if !l.started.CompareAndSwap(false, true) {
		return
	}
	l.Server = server.New(l.address, l.options...)
	go l.Server.Serve()
	l.started.Store(true)
}

func (l *Listener) Shutdown(ctx context.Context) error {
	if !l.started.CompareAndSwap(true, false) {
		return nil
	}
	l.Server.ExpelAllConns()
	err := l.Server.Shutdown(ctx)
	return err
}
