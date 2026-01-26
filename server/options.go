// Package server provides a server implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like connection management, broadcasting, and request-response mechanism.
package server

import (
	"context"
	"crypto/tls"
	"fmt"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/stats"
	"sutext.github.io/cable/xlog"
)

// ClosedHandler is called when a client connection is closed.
type ClosedHandler func(p *packet.Identity)

// ConnectHandler is called when a new client connects to the server.
// It returns a connection code indicating whether the connection is accepted.
type ConnectHandler func(ctx context.Context, p *packet.Connect) packet.ConnectCode

// MessageHandler is called when a message packet is received from a client.
type MessageHandler func(ctx context.Context, p *packet.Message, id *packet.Identity) error

// RequestHandler is called when a request packet is received from a client.
// It returns a response packet to be sent back to the client.
type RequestHandler func(ctx context.Context, p *packet.Request, id *packet.Identity) ([]byte, error)

// Network constants define the supported network protocols for the server.
const (
	NetworkWS   string = "ws"   // NetworkWS represents WebSocket protocol.
	NetworkWSS  string = "wss"  // NetworkWSS represents WebSocket Secure protocol.
	NetworkTCP  string = "tcp"  // NetworkTCP represents TCP protocol.
	NetworkTLS  string = "tls"  // NetworkTLS represents TCP with TLS protocol.
	NetworkUDP  string = "udp"  // NetworkUDP represents UDP protocol.
	NetworkQUIC string = "quic" // NetworkQUIC represents QUIC protocol.
)

// Option is a function type for configuring the server using builder pattern.
type Option struct {
	f func(*options) // function that modifies options
}

// options holds the configuration for the server.
type options struct {
	logger         *xlog.Logger   // logger for server logging
	network        string         // network protocol to use
	tlsConfig      *tls.Config    // TLS configuration for secure protocols
	quicConfig     *quic.Config   // QUIC configuration for QUIC protocol
	queueCapacity  int32          // capacity of the send queue
	statsHandler   stats.Handler  // handler for statistics events
	closeHandler   ClosedHandler  // handler for connection close events
	connectHandler ConnectHandler // handler for connection events
	messageHandler MessageHandler // handler for message events
	requestHandler RequestHandler // handler for request events
}

// NewOptions creates a new options instance with default values and applies the given options.
//
// Parameters:
// - opts: Configuration options to apply
//
// Returns:
// - *options: A new options instance with the given options applied
func NewOptions(opts ...Option) *options {
	var options = &options{
		logger:        xlog.With("GROUP", "SERVER"),
		network:       NetworkTCP,
		queueCapacity: 1024,
		closeHandler: func(p *packet.Identity) {
			// Default empty close handler
		},
		connectHandler: func(ctx context.Context, p *packet.Connect) packet.ConnectCode {
			// Default connect handler accepts all connections
			return packet.ConnectAccepted
		},
		messageHandler: func(ctx context.Context, p *packet.Message, id *packet.Identity) error {
			// Default message handler returns error if not implemented
			return fmt.Errorf("MessageHandler not implemented")
		},
		requestHandler: func(ctx context.Context, p *packet.Request, id *packet.Identity) ([]byte, error) {
			// Default request handler returns error if not implemented
			return nil, packet.StatusNotFound
		},
	}
	for _, o := range opts {
		o.f(options)
	}
	return options
}

// WithNetwork sets the network protocol for the server.
//
// Parameters:
// - network: Network protocol to use (TCP, UDP, WebSocket, QUIC)
//
// Returns:
// - Option: Configuration option for the server
func WithNetwork(network string) Option {
	return Option{f: func(o *options) { o.network = network }}
}

// WithTLSConfig sets the TLS configuration for secure protocols.
//
// Parameters:
// - config: TLS configuration to use
//
// Returns:
// - Option: Configuration option for the server
func WithTLSConfig(config *tls.Config) Option {
	return Option{f: func(o *options) { o.tlsConfig = config }}
}

// WithQUICConfig sets the QUIC configuration for QUIC network protocol.
//
// Parameters:
// - config: QUIC configuration to use
//
// Returns:
// - Option: Configuration option for the server
func WithQUICConfig(config *quic.Config) Option {
	return Option{f: func(o *options) { o.quicConfig = config }}
}

// WithLogger sets the custom logger for the server.
//
// Parameters:
// - logger: Logger to use for server logging
//
// Returns:
// - Option: Configuration option for the server
func WithLogger(logger *xlog.Logger) Option {
	return Option{f: func(o *options) { o.logger = logger }}
}

// WithClose sets the close handler for connection close events.
//
// Parameters:
// - handler: Close handler function
//
// Returns:
// - Option: Configuration option for the server
func WithClose(handler ClosedHandler) Option {
	return Option{f: func(o *options) { o.closeHandler = handler }}
}

// WithConnect sets the connect handler for new client connections.
//
// Parameters:
// - handler: Connect handler function
//
// Returns:
// - Option: Configuration option for the server
func WithConnect(handler ConnectHandler) Option {
	return Option{f: func(o *options) { o.connectHandler = handler }}
}

// WithMessage sets the message handler for incoming message packets.
//
// Parameters:
// - handler: Message handler function
//
// Returns:
// - Option: Configuration option for the server
func WithMessage(handler MessageHandler) Option {
	return Option{f: func(o *options) { o.messageHandler = handler }}
}

// WithRequest sets the request handler for incoming request packets.
//
// Parameters:
// - handler: Request handler function
//
// Returns:
// - Option: Configuration option for the server
func WithRequest(handler RequestHandler) Option {
	return Option{f: func(o *options) { o.requestHandler = handler }}
}
func WithStatsHandler(handler stats.Handler) Option {
	return Option{f: func(o *options) { o.statsHandler = handler }}
}

// WithSendQueue sets the capacity of the send queue.
//
// Parameters:
// - cap: Capacity of the send queue
//
// Returns:
// - Option: Configuration option for the server
func WithSendQueue(cap int32) Option {
	return Option{f: func(o *options) { o.queueCapacity = cap }}
}
