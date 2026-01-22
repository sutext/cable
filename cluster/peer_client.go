// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
package cluster

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/raft/v3/raftpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/stats"
	"sutext.github.io/cable/cluster/pb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

// peerClient represents a gRPC client for communicating with a peer broker.
// It manages the connection lifecycle and provides methods for sending messages and Raft commands.
type peerClient struct {
	id           uint64               // Unique ID of the peer broker
	mu           sync.Mutex           // Mutex for connection management
	rpc          pb.PeerServiceClient // gRPC client for peer service
	addr         string               // Address of the peer broker
	port         string               // Port of the peer broker (currently unused)
	conn         *grpc.ClientConn     // gRPC client connection
	ready        atomic.Bool          // Atomic flag indicating if the client is ready
	logger       *xlog.Logger         // Logger for peer client events
	statsHandler stats.Handler        // Stats handler for peer client
}

// newPeerClient creates a new peer client instance.
//
// Parameters:
// - id: Unique ID of the peer broker
// - addr: Address of the peer broker (host:port)
// - logger: Logger for peer client events
//
// Returns:
// - *peerClient: A new peer client instance
func newPeerClient(id uint64, addr string, logger *xlog.Logger) *peerClient {
	p := &peerClient{
		id:     id,
		addr:   addr,
		logger: logger,
	}
	return p
}

// connect establishes a connection to the peer broker. If the connection fails, it retries after 2 seconds.
// It uses a mutex to ensure thread safety during connection establishment.
func (p *peerClient) connect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ready.Load() {
		return
	}
	if err := p.reconnnect(); err != nil {
		p.logger.Error("connect to peer failed", xlog.Err(err), xlog.Peer(p.id))
		time.AfterFunc(time.Second*2, func() {
			p.connect()
		})
	} else {
		p.logger.Info("connect to peer success", xlog.Peer(p.id))
		p.ready.Store(true)
	}
}

// reconnnect establishes a new gRPC connection to the peer broker using configured gRPC settings.
// It closes any existing connection before creating a new one.
//
// Returns:
// - error: Error if connection establishment fails, nil otherwise
func (p *peerClient) reconnnect() error {
	config := `{
  "loadBalancingPolicy": "round_robin",
 "methodConfig": [{
    "name": [{"service": "pb.PeerService"}],
    "waitForReady": true,
    "timeout": "3s",
    "retryPolicy": {
      "maxAttempts": 3,
      "initialBackoff": "0.1s",
      "maxBackoff": "1s",
      "backoffMultiplier": 2,
      "retryableStatusCodes": ["UNAVAILABLE"]
    }
  }]
}`
	conn, err := grpc.NewClient(p.addr,
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				MaxDelay:   20 * time.Second,
				Multiplier: 1.2,
				Jitter:     0.2,
			},
		}),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithNoProxy(),
		grpc.WithDefaultServiceConfig(config),
		grpc.WithStatsHandler(p.statsHandler),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = conn
	p.rpc = pb.NewPeerServiceClient(conn)
	return nil
}

// Close closes the peer client connection and releases resources.
// It uses atomic compare-and-swap to ensure the connection is only closed once.
func (p *peerClient) Close() {
	if p.ready.CompareAndSwap(true, false) {
		if p.conn != nil {
			p.conn.Close()
			p.conn = nil
		}
		p.logger.Info("peer client closed", xlog.Peer(p.id))
	}
}

// isReady checks if the peer client is ready for communication.
// It atomically checks the ready flag to ensure thread-safe access.
//
// Returns:
// - bool: True if the client is ready, false otherwise
func (p *peerClient) isReady() bool {
	return p.ready.Load()
}

// updateAddr updates the address of the peer broker and reconnects if the address has changed.
// It marks the client as not ready and initiates a new connection.
//
// Parameters:
// - addr: New address of the peer broker (host:port)
func (p *peerClient) updateAddr(addr string) {
	if p.addr == addr {
		return
	}
	p.logger.Warn("peer ip updated", xlog.Str("addr", addr))
	p.addr = addr
	p.ready.Store(false)
	p.connect()
}

// sendToAll sends a message to all clients connected to the peer broker.
//
// Parameters:
// - ctx: Context for the request
// - m: Message to send to all clients
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
// - error: Error if the peer is not ready or sending fails, nil otherwise
func (p *peerClient) sendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error) {
	if !p.isReady() {
		return 0, 0, xerr.PeerNotReady
	}
	data, err := coder.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	req := &pb.MessageReq{
		Message: data,
	}
	resp, err := p.rpc.SendToAll(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}

// sendToTargets sends a message to specific target clients on the peer broker.
//
// Parameters:
// - ctx: Context for the request
// - m: Message to send to target clients
// - tragets: Map of client IDs to network protocols specifying the target clients
//
// Returns:
// - int32: Total number of targets attempted to send to
// - int32: Number of successful sends
// - error: Error if the peer is not ready or sending fails, nil otherwise
func (p *peerClient) sendToTargets(ctx context.Context, m *packet.Message, tragets map[string]string) (total, success int32, err error) {
	if !p.isReady() {
		return 0, 0, xerr.PeerNotReady
	}
	data, err := coder.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	req := &pb.MessageReq{
		Message: data,
		Targets: tragets,
	}
	resp, err := p.rpc.SendToTargets(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}

// isOnline checks if any of the specified target clients are online on the peer broker.
//
// Parameters:
// - ctx: Context for the request
// - targets: Map of client IDs to network protocols specifying the target clients
//
// Returns:
// - bool: True if any target client is online, false otherwise
// - error: Error if the peer is not ready or checking fails, nil otherwise
func (p *peerClient) isOnline(ctx context.Context, targets map[string]string) (bool, error) {
	if !p.isReady() {
		return false, xerr.PeerNotReady
	}
	resp, err := p.rpc.IsOnline(ctx, &pb.IsOnlineReq{Targets: targets})
	if err != nil {
		return false, err
	}
	return resp.Online, nil
}

// kickConn kicks the specified target clients from the peer broker.
//
// Parameters:
// - ctx: Context for the request
// - targets: Map of client IDs to network protocols specifying the target clients to kick
//
// Returns:
// - error: Error if the peer is not ready or kicking fails, nil otherwise
func (p *peerClient) kickConn(ctx context.Context, targets map[string]string) error {
	if !p.isReady() {
		return xerr.PeerNotReady
	}
	_, err := p.rpc.KickConn(ctx, &pb.KickConnReq{Targets: targets})
	return err
}

// inspect returns the status of the peer broker.
//
// Parameters:
// - ctx: Context for the request
//
// Returns:
// - *pb.Status: Status information of the peer broker
// - error: Error if the peer is not ready or inspecting fails, nil otherwise
func (p *peerClient) inspect(ctx context.Context) (*pb.Status, error) {
	if !p.isReady() {
		return nil, xerr.PeerNotReady
	}
	return p.rpc.Inspect(ctx, &pb.Empty{})
}

// sendRaftMessage sends a Raft message to the peer broker.
// This is used for Raft consensus communication between cluster nodes.
//
// Parameters:
// - ctx: Context for the request
// - msg: Raft message to send to the peer
//
// Returns:
// - error: Error if the peer is not ready or sending fails, nil otherwise
func (p *peerClient) sendRaftMessage(ctx context.Context, msg raftpb.Message) error {
	if !p.isReady() {
		return xerr.PeerNotReady
	}
	data, err := msg.Marshal()
	if err != nil {
		return err
	}
	req := &pb.RaftMessage{
		Data: data,
	}
	_, err = p.rpc.SendRaftMessage(ctx, req)
	return err
}
