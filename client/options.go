// Package client provides a client implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like automatic reconnection, heartbeat detection, and request-response mechanism.
package client

import (
	"crypto/tls"
	"time"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

// Network constants define the supported network protocols.
const (
	// NetworkWS represents WebSocket protocol.
	NetworkWS string = "ws"
	// NetworkWSS represents WebSocket Secure protocol.
	NetworkWSS string = "wss"
	// NetworkTCP represents TCP protocol.
	NetworkTCP string = "tcp"
	// NetworkTLS represents TCP with TLS protocol.
	NetworkTLS string = "tls"
	// NetworkUDP represents UDP protocol.
	NetworkUDP string = "udp"
	// NetworkQUIC represents QUIC protocol.
	NetworkQUIC string = "quic"
)

// Handler defines the callback methods for client events.
type Handler interface {
	// OnStatus is called when the client status changes.
	OnStatus(status Status)
	// OnMessage is called when a message packet is received.
	OnMessage(p *packet.Message) error
	// OnRequest is called when a request packet is received and expects a response.
	OnRequest(p *packet.Request) (*packet.Response, error)
}

// emptyHandler is a default implementation of Handler that does nothing.
type emptyHandler struct{}

// OnStatus implements the Handler interface with empty implementation.
func (h *emptyHandler) OnStatus(status Status) {}

// OnMessage implements the Handler interface with empty implementation.
func (h *emptyHandler) OnMessage(p *packet.Message) error {
	return nil
}

// OnRequest implements the Handler interface with empty implementation.
func (h *emptyHandler) OnRequest(p *packet.Request) (*packet.Response, error) {
	return nil, nil
}

// Options holds the configuration for the client.
type Options struct {
	id                *packet.Identity // client identity
	logger            *xlog.Logger     // logger for client logging
	handler           Handler          // event handler for client events
	retrier           *Retrier         // retry mechanism for reconnections
	tlsConfig         *tls.Config      // TLS configuration (if using TLS)
	quicConfig        *quic.Config     // QUIC configuration (if using QUIC)
	pingTimeout       time.Duration    // timeout for ping responses
	pingInterval      time.Duration    // interval between ping messages
	writeTimeout      time.Duration    // timeout for write operations
	requestTimeout    time.Duration    // timeout for request operations
	messageTimeout    time.Duration    // timeout for message operations
	sendQueueCapacity int32            // capacity of the send queue
}

// Option is a function type for configuring the client using builder pattern.
type Option struct {
	f func(*Options) // function that modifies Options
}

// newOptions creates a new Options instance with default values and applies the given options.
func newOptions(options ...Option) *Options {
	opts := &Options{
		logger:            xlog.With("GROUP", "CLIENT"),
		handler:           &emptyHandler{},
		retrier:           NewRetrier(10, backoff.Exponential(time.Second, 1.5)),
		pingTimeout:       time.Second * 5,
		pingInterval:      time.Second * 60,
		writeTimeout:      time.Second * 5,
		requestTimeout:    time.Second * 5,
		messageTimeout:    time.Second * 5,
		sendQueueCapacity: 1024,
		tlsConfig: &tls.Config{ // default TLS configuration, using self-signed certificate
			InsecureSkipVerify: true,
		},
		quicConfig: &quic.Config{ // default QUIC configuration
			MaxIdleTimeout: 5 * time.Second,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	for _, o := range options {
		o.f(opts)
	}
	return opts
}
func WithID(id *packet.Identity) Option {
	return Option{f: func(o *Options) {
		o.id = id
	}}
}

// WithPing sets the ping timeout and interval for keep-alive mechanism.
func WithPing(timeout, interval time.Duration) Option {
	return Option{f: func(o *Options) {
		o.pingTimeout = timeout
		o.pingInterval = interval
	}}
}

// WithLogger sets the custom logger for the client.
func WithLogger(logger *xlog.Logger) Option {
	return Option{f: func(o *Options) {
		o.logger = logger
	}}
}

// WithRetrier sets the custom retrier for reconnection attempts.
func WithRetrier(retrier *Retrier) Option {
	return Option{f: func(o *Options) {
		o.retrier = retrier
	}}
}

// WithTLSConfig sets the TLS configuration for TLS network protocol.
func WithTLSConfig(config *tls.Config) Option {
	return Option{f: func(o *Options) {
		o.tlsConfig = config
	}}
}

// WithQuicConfig sets the QUIC configuration for QUIC network protocol.
func WithQuicConfig(config *quic.Config) Option {
	return Option{f: func(o *Options) {
		o.quicConfig = config
	}}
}

// WithKeepAlive sets the keep-alive timeout and interval (alias for WithPing).
func WithKeepAlive(timeout, interval time.Duration) Option {
	return Option{f: func(o *Options) {
		o.pingTimeout = timeout
		o.pingInterval = interval
	}}
}

// WithHandler sets the custom event handler for client events.
func WithHandler(handler Handler) Option {
	return Option{f: func(o *Options) {
		o.handler = handler
	}}
}

// WithWriteTimeout sets the timeout for write operations.
func WithWriteTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.writeTimeout = timeout
	}}
}

// WithRequestTimeout sets the timeout for request operations.
func WithRequestTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.requestTimeout = timeout
	}}
}

// WithMessageTimeout sets the timeout for message operations.
func WithMessageTimeout(timeout time.Duration) Option {
	return Option{f: func(o *Options) {
		o.messageTimeout = timeout
	}}
}

// WithSendQueue sets the capacity of the send queue.
func WithSendQueue(capacity int32) Option {
	return Option{f: func(o *Options) {
		o.sendQueueCapacity = capacity
	}}
}
