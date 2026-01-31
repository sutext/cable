// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
package cluster

import (
	"context"
	"fmt"
	"sync/atomic"

	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

// Listener wraps a server instance and manages its lifecycle within the cluster.
type Listener struct {
	server.Server                 // Underlying server instance
	logger        *xlog.Logger    // Logger for listener events
	started       atomic.Bool     // Flag indicating if the listener is started (atomic)
	network       string          // Network protocol (TCP, UDP, WebSocket, QUIC)
	port          uint16          // Port to listen on
	options       []server.Option // Server options
	autoStart     bool            // Whether to automatically start the listener
}

// NewListener creates a new Listener instance.
//
// Parameters:
// - network: Network protocol to use (TCP, UDP, WebSocket, QUIC)
// - port: Port to listen on
// - autoStart: Whether to automatically start the listener
// - opts: Server options to apply
//
// Returns:
// - *Listener: A new Listener instance
func NewListener(network string, port uint16, autoStart bool, opts ...server.Option) *Listener {
	return &Listener{
		port:      port,
		logger:    xlog.With("LISTENER", network),
		network:   network,
		options:   append(opts, server.WithNetwork(network)),
		autoStart: autoStart,
	}
}

// AddOptions adds additional server options to the listener.
//
// Parameters:
// - opts: Server options to add
func (l *Listener) AddOptions(opts ...server.Option) {
	l.options = append(l.options, opts...)
}

// ISStarted checks if the listener is started.
//
// Returns:
// - bool: True if the listener is started, false otherwise
func (l *Listener) ISStarted() bool {
	return l.started.Load()
}

// Start starts the listener and underlying server.
// If the listener is already started, it does nothing.
//
// Parameters:
// - opts: Additional server options to apply when starting
func (l *Listener) Start() {
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

// Shutdown shuts down the listener and underlying server.
// If the listener is not started, it does nothing.
//
// Parameters:
// - ctx: Context for the shutdown operation
//
// Returns:
// - error: Error if shutdown fails, nil otherwise
func (l *Listener) Shutdown(ctx context.Context) error {
	if !l.started.CompareAndSwap(true, false) {
		return nil
	}
	l.Server.ExpelAllConns()
	err := l.Server.Shutdown(ctx)
	return err
}
