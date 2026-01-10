package cluster

import (
	"context"
	"sync/atomic"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/internal/discovery"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Cluster interface {
	AddBroker(ctx context.Context, broker uint64, addr string) error
	RemoveBroker(ctx context.Context, broker uint64) error
}

type cluster struct {
	raft            raft.Node
	size            int32
	ready           atomic.Bool
	peers           safe.RMap[uint64, *peerClient]
	broker          *broker
	logger          *xlog.Logger
	stoped          chan struct{}
	leader          atomic.Uint64
	storage         *raft.MemoryStorage
	discovery       discovery.Discovery
	confState       *raftpb.ConfState
	appliedIndex    uint64
	snapshotIndex   uint64
	confChangeCount uint64
}

func newCluster(broker *broker, initSize int32) *cluster {
	c := &cluster{
		size:      initSize,
		broker:    broker,
		stoped:    make(chan struct{}),
		logger:    broker.logger,
		discovery: discovery.New(broker.id, broker.peerPort),
	}
	c.discovery.OnRequest(func(id uint64, addr string) {
		c.addPeer(id, addr)
	})
	return c

}
func (c *cluster) Start() {
	go func() {
		if err := c.discovery.Serve(); err != nil {
			panic(err)
		}
	}()
	time.AfterFunc(time.Millisecond*100, c.autoDiscovery)
}
func (c *cluster) Stop() {
	if err := c.discovery.Shutdown(); err != nil {
		c.logger.Error("muticast shutdown", xlog.Err(err))
	}
	if c.raft != nil {
		c.raft.Stop()
		close(c.stoped)
	}
}
func (c *cluster) Status() (raft.Status, error) {
	if c.raft == nil {
		return raft.Status{}, xerr.RaftNodeNotReady
	}
	return c.raft.Status(), nil
}
func (c *cluster) RaftLogSize() uint64 {
	firstIndex, _ := c.storage.FirstIndex()
	lastIndex, _ := c.storage.LastIndex()
	return lastIndex - firstIndex + 1
}
func (c *cluster) AddBroker(ctx context.Context, brokerID uint64, addr string) error {
	c.addPeer(brokerID, addr)
	return nil
}

func (c *cluster) RemoveBroker(ctx context.Context, brokerID uint64) error {
	return c.removeNode(ctx, brokerID)
}
func (c *cluster) GetPeer(id uint64) (*peerClient, bool) {
	if p, ok := c.peers.Get(id); ok {
		return p, true
	}
	return nil, false
}
func (c *cluster) RangePeers(f func(uint64, *peerClient)) {
	c.peers.Range(func(key uint64, value *peerClient) bool {
		f(key, value)
		return true
	})
}
func (c *cluster) Process(ctx context.Context, m raftpb.Message) error {
	if c.raft == nil {
		return xerr.RaftNodeNotReady
	}
	return c.raft.Step(ctx, m)
}
func (c *cluster) IsLeader() bool {
	return c.leader.Load() == c.broker.id
}
func (c *cluster) Leader() uint64 {
	return c.leader.Load()
}
func (c *cluster) IsReady() bool {
	return c.ready.Load()
}
func (c *cluster) IsIDRemoved(id uint64) bool {
	_, ok := c.peers.Get(id)
	return !ok
}
func (c *cluster) ReportUnreachable(id uint64) {
	c.raft.ReportUnreachable(id)
}
func (c *cluster) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	c.raft.ReportSnapshot(id, status)
}
func (c *cluster) addPeer(id uint64, addr string) {
	if id == c.broker.id {
		return
	}
	if p, ok := c.peers.Get(id); ok {
		p.updateAddr(addr)
		return
	}
	peer := newPeerClient(id, addr, c.broker.logger)
	c.peers.Set(id, peer)
	peer.connect()
	if c.peers.Len() > c.size-1 && c.IsLeader() {
		c.addNode(context.Background(), id)
	}
}
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
		c.addPeer(id, addr)
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
