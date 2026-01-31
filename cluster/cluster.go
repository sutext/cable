// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
package cluster

import (
	"context"
	"sync/atomic"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"google.golang.org/grpc/stats"
	"sutext.github.io/cable/internal/discovery"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

// Cluster defines the interface for cluster management operations.
type Cluster interface {
	Size() int32
	// IsReady checks if the cluster is ready for operations.
	//
	// Returns:
	// - bool: True if the cluster is ready, false otherwise
	IsReady() bool
	// AddBroker adds a new broker to the cluster.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - id: Unique ID of the broker to add
	// - addr: Address of the broker to add
	//
	// Returns:
	// - error: Error if adding the broker fails, nil otherwise
	AddBroker(ctx context.Context, id uint64, addr string) error
	// KickBroker removes a broker from the cluster.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - id: Unique ID of the broker to remove
	//
	// Returns:
	// - error: Error if removing the broker fails, nil otherwise
	KickBroker(ctx context.Context, id uint64) error
}

// cluster implements the Cluster interface and manages the raft consensus group.
type cluster struct {
	raft            raft.Node                      // Raft node instance
	size            int32                          // Expected cluster size
	ready           atomic.Bool                    // Flag indicating if the cluster is ready (atomic)
	peers           safe.RMap[uint64, *peerClient] // Map of peer brokers (ID -> peerClient)
	broker          *broker                        // Local broker instance
	logger          *xlog.Logger                   // Logger for cluster events
	stoped          chan struct{}                  // Channel to signal cluster stop
	leader          atomic.Uint64                  // Current leader ID (atomic)
	storage         *raft.MemoryStorage            // Raft log storage
	confState       *raftpb.ConfState              // Raft configuration state
	discovery       discovery.Discovery            // Service discovery for peer brokers
	appliedIndex    uint64                         // Last applied Raft log index
	snapshotIndex   uint64                         // Last snapshot index
	confChangeCount uint64                         // Number of configuration changes
	peerStats       stats.Handler                  // Stats handler for peer client events
}

// newCluster creates a new cluster instance.
//
// Parameters:
// - broker: Local broker instance
// - size: Expected cluster size
// - port: Port for peer-to-peer communication
//
// Returns:
// - *cluster: A new cluster instance
func newCluster(broker *broker, opts *options) *cluster {
	c := &cluster{
		size:      opts.clusterSize,
		broker:    broker,
		logger:    broker.logger,
		stoped:    make(chan struct{}),
		peerStats: opts.grpcStatsHandler.Client,
		discovery: discovery.New(broker.id, opts.peerPort),
	}
	c.discovery.OnRequest(func(id uint64, addr string) {
		c.AddBroker(context.Background(), id, addr)
	})
	return c
}

// Stop stops the cluster and cleans up resources.
func (c *cluster) Stop() {
	if err := c.discovery.Shutdown(); err != nil {
		c.logger.Error("muticast shutdown", xlog.Err(err))
	}
	if c.raft != nil {
		c.raft.Stop()
		close(c.stoped)
	}
}

// Start starts the cluster and begins the discovery process.
func (c *cluster) Start() {
	go func() {
		if err := c.discovery.Serve(); err != nil {
			c.logger.Info("discovery serve stoped", xlog.Err(err))
		}
	}()
	time.AfterFunc(time.Millisecond*100, c.autoDiscovery)
}
func (c *cluster) Size() int32 {
	return c.size
}

// Status returns the current status of the Raft node.
//
// Returns:
// - raft.Status: Current Raft node status
// - error: Error if the Raft node is not ready, nil otherwise
func (c *cluster) Status() (raft.Status, error) {
	if c.raft == nil {
		return raft.Status{}, xerr.RaftNodeNotReady
	}
	return c.raft.Status(), nil
}

// RaftLogSize returns the number of entries in the Raft log.
//
// Returns:
// - uint64: Number of entries in the Raft log
func (c *cluster) RaftLogSize() uint64 {
	firstIndex, _ := c.storage.FirstIndex()
	lastIndex, _ := c.storage.LastIndex()
	return lastIndex - firstIndex + 1
}

// AddBroker adds a new broker to the cluster.
//
// Parameters:
// - ctx: Context for the operation
// - id: Unique ID of the broker to add
// - addr: Address of the broker to add
//
// Returns:
// - error: Error if adding the broker fails, nil otherwise
func (c *cluster) AddBroker(ctx context.Context, id uint64, addr string) error {
	if id == c.broker.id {
		return nil
	}
	if p, ok := c.peers.Get(id); ok {
		p.updateAddr(addr)
		return nil
	}
	peer := newPeerClient(id, addr, c.logger, c.peerStats)
	c.peers.Set(id, peer)
	peer.connect()
	if c.peers.Len() == c.size-1 {
		return nil
	}
	if !c.IsLeader() {
		return nil
	}
	return c.addNode(ctx, id)
}

// KickBroker removes a broker from the cluster.
//
// Parameters:
// - ctx: Context for the operation
// - id: Unique ID of the broker to remove
//
// Returns:
// - error: Error if removing the broker fails, nil otherwise
func (c *cluster) KickBroker(ctx context.Context, id uint64) error {
	if id == c.broker.id {
		c.ready.Store(false)
	}
	return c.removeNode(ctx, id)
}

// GetPeer returns a peer client by ID.
//
// Parameters:
// - id: Unique ID of the peer to get
//
// Returns:
// - *peerClient: Peer client if found, nil otherwise
// - bool: True if the peer exists, false otherwise
func (c *cluster) GetPeer(id uint64) (*peerClient, bool) {
	if p, ok := c.peers.Get(id); ok {
		return p, true
	}
	return nil, false
}

// RangePeers iterates over all peer clients and applies the given function.
//
// Parameters:
// - f: Function to apply to each peer client
func (c *cluster) RangePeers(f func(uint64, *peerClient) bool) {
	c.peers.Range(func(key uint64, value *peerClient) bool {
		return f(key, value)
	})
}

// Process processes a Raft message.
//
// Parameters:
// - ctx: Context for the operation
// - m: Raft message to process
//
// Returns:
// - error: Error if processing fails, nil otherwise
func (c *cluster) Process(ctx context.Context, m raftpb.Message) error {
	if c.raft == nil {
		return xerr.RaftNodeNotReady
	}
	return c.raft.Step(ctx, m)
}

// IsLeader checks if the local broker is the cluster leader.
//
// Returns:
// - bool: True if the local broker is the leader, false otherwise
func (c *cluster) IsLeader() bool {
	return c.leader.Load() == c.broker.id
}

// Leader returns the ID of the current cluster leader.
//
// Returns:
// - uint64: ID of the current cluster leader
func (c *cluster) Leader() uint64 {
	return c.leader.Load()
}

// IsReady checks if the cluster is ready.
//
// Returns:
// - bool: True if the cluster is ready, false otherwise
func (c *cluster) IsReady() bool {
	return c.ready.Load()
}

// IsIDRemoved checks if a broker ID has been removed from the cluster.
//
// Parameters:
// - id: Broker ID to check
//
// Returns:
// - bool: True if the broker ID has been removed, false otherwise
func (c *cluster) IsIDRemoved(id uint64) bool {
	_, ok := c.peers.Get(id)
	return !ok
}

// ReportUnreachable reports that a peer is unreachable.
//
// Parameters:
// - id: ID of the unreachable peer
func (c *cluster) ReportUnreachable(id uint64) {
	c.raft.ReportUnreachable(id)
}

// ReportSnapshot reports the status of a snapshot operation.
//
// Parameters:
// - id: ID of the peer that received the snapshot
// - status: Status of the snapshot operation
func (c *cluster) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	c.raft.ReportSnapshot(id, status)
}

// autoDiscovery performs automatic discovery of peer brokers.
// It requests peer information and starts the Raft cluster when the expected number of peers is found.
func (c *cluster) autoDiscovery() {
	m, err := c.discovery.Request()
	if err != nil {
		c.logger.Error("failed to sync broker", xlog.Err(err))
	}
	if len(m) == 0 {
		c.logger.Warn("Auto discovery got empty endpoints")
		go c.autoDiscovery()
		return
	}
	for id, addr := range m {
		c.AddBroker(context.Background(), id, addr)
	}
	if c.peers.Len() < c.size-1 {
		c.logger.Warn("Auto discovery got less than cluster size", xlog.I32("clusterSize", c.size), xlog.I32("peers", c.peers.Len()))
		go c.autoDiscovery()
	} else if c.peers.Len() == c.size-1 {
		c.startRaft(false)
	} else {
		c.startRaft(true)
	}
}
