package cluster

import (
	"context"
	"fmt"
	"sync/atomic"

	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type Listener struct {
	server.Server
	logger    *xlog.Logger
	started   atomic.Bool
	network   string
	port      uint16
	options   []server.Option
	autoStart bool
}

func NewListener(network string, port uint16, autoStart bool, opts ...server.Option) *Listener {
	return &Listener{
		port:      port,
		logger:    xlog.With("LISTENER", network),
		network:   network,
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
	l.Server = server.New(fmt.Sprintf(":%d", l.port), l.options...)
	go func() {
		err := l.Server.Serve()
		if err != nil {
			l.logger.Info("server stoped", xlog.Str("network", l.network), xlog.Err(err))
		}
	}()
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
