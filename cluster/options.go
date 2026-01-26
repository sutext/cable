// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
package cluster

import (
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/stats"
)

// Handler defines the callback methods for cluster events.
type Handler interface {
	// OnUserClosed is called when a client connection is closed.
	//
	// Parameters:
	// - id: Identity of the closed connection
	OnUserClosed(id *packet.Identity)
	// OnUserConnect is called when a new client connects to the cluster.
	// It returns a connection code indicating whether the connection is accepted.
	//
	// Parameters:
	// - p: Connect packet from the client
	//
	// Returns:
	// - packet.ConnectCode: Connection result code
	OnUserConnect(ctx context.Context, p *packet.Connect) packet.ConnectCode
	// OnUserMessage is called when a message packet is received from a client.
	//
	// Parameters:
	// - ctx: Context for the request
	// - p: Message packet received
	// - id: Identity of the client that sent the message
	//
	// Returns:
	// - error: Error if message handling fails, nil otherwise
	OnUserMessage(ctx context.Context, p *packet.Message, id *packet.Identity) error
	// GetUserChannels returns the list of channels a user has joined.
	//
	// Parameters:
	// - uid: User ID to get channels for
	//
	// Returns:
	// - map[string]string: List of channels the user has joined
	// - error: Error if getting channels fails, nil otherwise
	GetUserChannels(uid string) (channels map[string]string, err error) //uid join channels
}

// emptyHandler is a default implementation of Handler that does nothing.
type emptyHandler struct{}

// OnUserClosed implements the Handler interface with empty implementation.
func (h *emptyHandler) OnUserClosed(id *packet.Identity) {
}

// OnUserConnect implements the Handler interface with default implementation that accepts all connections.
func (h *emptyHandler) OnUserConnect(ctx context.Context, p *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}

// OnUserMessage implements the Handler interface with empty implementation.
func (h *emptyHandler) OnUserMessage(ctx context.Context, p *packet.Message, id *packet.Identity) error {
	return nil
}

// GetUserChannels implements the Handler interface with empty implementation.
func (h *emptyHandler) GetUserChannels(uid string) (channels map[string]string, err error) {
	return nil, nil
}

// options holds the configuration for the cluster.
type options struct {
	handler          Handler           // Handler for cluster events
	brokerID         uint64            // Unique ID for the broker
	peerPort         uint16            // Port for peer-to-peer communication
	listeners        []*Listener       // List of listeners for client connections
	clusterSize      int32             // Initial cluster size
	statsHandler     stats.Handler     // Stats handler for the cluster
	grpcStatsHandler stats.GrpcHandler // gRPC stats handler for the cluster
}

// newOptions creates a new options instance with default values and applies the given options.
//
// Parameters:
// - opts: Configuration options to apply
//
// Returns:
// - *options: A new options instance with the given options applied
func newOptions(opts ...Option) *options {
	options := &options{
		handler:     &emptyHandler{},
		peerPort:    4567,
		clusterSize: 3,
	}
	for _, opt := range opts {
		opt.f(options)
	}
	return options
}

// Option is a function type for configuring the cluster using builder pattern.
type Option struct {
	f func(*options) // function that modifies options
}

// WithHandler sets the handler for cluster events.
//
// Parameters:
// - h: Handler for cluster events
//
// Returns:
// - Option: Configuration option for the cluster
func WithHandler(h Handler) Option {
	return Option{func(o *options) {
		o.handler = h
	}}
}

// WithPeerPort sets the port for peer-to-peer communication.
//
// Parameters:
// - port: Peer-to-peer port number
//
// Returns:
// - Option: Configuration option for the cluster
func WithPeerPort(port uint16) Option {
	return Option{func(o *options) {
		o.peerPort = port
	}}
}

// WithListeners sets the list of listeners for client connections.
//
// Parameters:
// - listerners: List of listeners
//
// Returns:
// - Option: Configuration option for the cluster
func WithListeners(listerners []*Listener) Option {
	return Option{func(o *options) {
		o.listeners = listerners
	}}
}

// WithBrokerID sets the unique ID for the broker.
//
// Parameters:
// - id: Broker ID
//
// Returns:
// - Option: Configuration option for the cluster
func WithBrokerID(id uint64) Option {
	return Option{func(o *options) {
		o.brokerID = id
	}}
}

// WithClusterSize sets the initial cluster size.
//
// Parameters:
// - size: Initial cluster size
//
// Returns:
// - Option: Configuration option for the cluster
func WithClusterSize(size int32) Option {
	return Option{func(o *options) {
		o.clusterSize = size
	}}
}

// WithStatsHandler sets the stats handler for the cluster.
//
// Parameters:
// - handler: Stats handler
//
// Returns:
// - Option: Configuration option for the cluster
func WithStatsHandler(handler stats.Handler) Option {
	return Option{func(o *options) {
		o.statsHandler = handler
	}}
}

// WithGrpcStatsHandler sets the gRPC stats handler for the cluster.
//
// Parameters:
// - handler: gRPC stats handler
//
// Returns:
// - Option: Configuration option for the cluster
func WithGrpcStatsHandler(handler stats.GrpcHandler) Option {
	return Option{func(o *options) {
		o.grpcStatsHandler = handler
	}}
}

// getBrokerID returns a unique ID for the broker.
// It tries to derive an ID from the hostname, otherwise generates a random ID.
//
// Returns:
// - uint64: Unique broker ID
func getBrokerID() uint64 {
	if hostname, err := os.Hostname(); err == nil {
		strs := strings.Split(hostname, "-")
		if len(strs) > 1 {
			if id, err := strconv.ParseUint(strs[len(strs)-1], 10, 64); err == nil {
				return id + 10000
			}
		}
	}
	return rand.Uint64()
}
