package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type snapshot struct {
	UserClients    map[uint64]map[string]map[string]string   `json:"user_clients"`
	ChannelClients map[uint64]map[string]map[string]string   `json:"channel_clients"`
	ClientChannels map[uint64]map[string]map[string]struct{} `json:"client_channels"`
}
type optype uint8

const (
	optypeUserOpened optype = iota
	optypeUserClosed
	optypeJoinChannel
	optypeLeaveChannel
)

type opdata interface {
	coder.Codable
	opt() optype
}

type userOpenedOp struct {
	uid      string
	cid      string
	net      string
	nodeID   uint64
	channels []string
}

func (op *userOpenedOp) opt() optype {
	return optypeUserOpened
}
func (op *userOpenedOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteString(op.cid)
	e.WriteString(op.net)
	e.WriteUInt64(op.nodeID)
	e.WriteStrings(op.channels)
	return nil
}
func (op *userOpenedOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.cid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.net, err = d.ReadString()
	if err != nil {
		return err
	}
	op.nodeID, err = d.ReadUInt64()
	if err != nil {
		return err
	}
	op.channels, err = d.ReadStrings()
	return err
}

type userClosedOp struct {
	uid    string
	cid    string
	nodeID uint64
}

func (op *userClosedOp) opt() optype {
	return optypeUserClosed
}
func (op *userClosedOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteString(op.cid)
	e.WriteUInt64(op.nodeID)
	return nil
}
func (op *userClosedOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.cid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.nodeID, err = d.ReadUInt64()
	return err
}

type joinChannelOp struct {
	uid      string
	channels []string
}

func (op *joinChannelOp) opt() optype {
	return optypeJoinChannel
}
func (op *joinChannelOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteStrings(op.channels)
	return nil
}
func (op *joinChannelOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.channels, err = d.ReadStrings()
	return err
}

type leaveChannelOp struct {
	uid      string
	channels []string
}

func (op *leaveChannelOp) opt() optype {
	return optypeLeaveChannel
}
func (op *leaveChannelOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteStrings(op.channels)
	return nil
}
func (op *leaveChannelOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.channels, err = d.ReadStrings()
	return err
}

func (c *cluster) SubmitOperation(ctx context.Context, op opdata) error {
	if c.raft == nil {
		return xerr.RaftNodeNotReady
	}
	ec := coder.NewEncoder()
	ec.WriteUInt8(uint8(op.opt()))
	op.WriteTo(ec)
	c.raft.Propose(ctx, ec.Bytes())
	return nil
}
func (c *cluster) addNode(ctx context.Context, id uint64) error {
	if c.raft == nil {
		return xerr.RaftNodeNotReady
	}
	c.confChangeCount++
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeAddNode,
		NodeID: id,
		ID:     c.confChangeCount,
	}
	return c.raft.ProposeConfChange(ctx, cc)
}
func (c *cluster) removeNode(ctx context.Context, id uint64) error {
	if c.raft == nil {
		return xerr.RaftNodeNotReady
	}
	c.confChangeCount++
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: id,
		ID:     c.confChangeCount,
	}
	return c.raft.ProposeConfChange(ctx, cc)
}
func (c *cluster) startRaft(join bool) {
	if c.raft != nil {
		return
	}
	storage := raft.NewMemoryStorage()
	c.storage = storage
	conf := &raft.Config{
		ID:                        c.broker.id,
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   storage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
	}
	if join {
		c.raft = raft.RestartNode(conf)
	} else {
		initPeers := make([]raft.Peer, c.size)
		initPeers[0] = raft.Peer{ID: c.broker.id}
		c.peers.Range(func(key uint64, value *peerClient) bool {
			initPeers = append(initPeers, raft.Peer{ID: key})
			return true
		})
		c.raft = raft.StartNode(conf, initPeers)
	}
	go c.raftLoop()
}

func (c *cluster) raftLoop() {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-c.stoped:
			return
		case <-ticker.C:
			c.raft.Tick()
		case rd := <-c.raft.Ready():
			if !raft.IsEmptyHardState(rd.HardState) {
				if err := c.storage.SetHardState(rd.HardState); err != nil {
					c.logger.Error("Failed to set hard state", xlog.Err(err))
				}
			}
			if !raft.IsEmptySnap(rd.Snapshot) {
				if err := c.storage.ApplySnapshot(rd.Snapshot); err != nil {
					c.logger.Error("Failed to apply snapshot", xlog.Err(err))
				}
				c.applySnapshot(rd.Snapshot)
			}
			if len(rd.Entries) > 0 {
				if err := c.storage.Append(rd.Entries); err != nil {
					c.logger.Error("Failed to append entries", xlog.Err(err))
				}
			}
			if len(rd.CommittedEntries) > 0 {
				c.applyEntries(rd.CommittedEntries)
			}
			if rd.SoftState != nil {
				c.applySoftState(rd.SoftState)
			}
			if len(rd.Messages) > 0 {
				c.sendRaftMessags(rd.Messages)
			}
			c.attemptSnapshot()
			c.raft.Advance()
		}
	}
}
func (c *cluster) sendRaftMessags(msgs []raftpb.Message) {
	for _, msg := range msgs {
		if peer, ok := c.peers.Get(msg.To); ok {
			if msg.Type == raftpb.MsgSnap {
				msg.Snapshot.Metadata.ConfState = *c.confState
			}
			if err := peer.sendRaftMessage(context.Background(), msg); err != nil {
				c.raft.ReportUnreachable(msg.To)
				if msg.Type == raftpb.MsgSnap {
					c.raft.ReportSnapshot(msg.To, raft.SnapshotFailure)
				}
				c.logger.Error("Failed to send raft message to peer", xlog.Err(err))
			} else {
				if msg.Type == raftpb.MsgSnap {
					c.raft.ReportSnapshot(msg.To, raft.SnapshotFinish)
				}
			}
		}
	}
}
func (c *cluster) applySoftState(ss *raft.SoftState) {
	newLeader := ss.Lead
	if c.leader.Load() != newLeader {
		c.leader.Store(newLeader)
		c.logger.Info("New leader elected", xlog.U64("leader", newLeader))
	}
	c.ready.Store(true)
}
func (c *cluster) applyEntries(entries []raftpb.Entry) {
	for _, entry := range entries {
		switch entry.Type {
		case raftpb.EntryNormal:
			if len(entry.Data) > 0 {
				c.applyRaftOp(entry.Data)
			}
		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(entry.Data); err != nil {
				c.logger.Error("Failed to unmarshal conf change", xlog.Err(err))
				return
			}
			c.confState = c.raft.ApplyConfChange(cc)
			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				c.size = c.peers.Len() + 1
			case raftpb.ConfChangeRemoveNode:
				c.applyRemoveNode(cc.NodeID)
			}
		}
	}
	c.appliedIndex = entries[len(entries)-1].Index
}

func (c *cluster) applyRaftOp(data []byte) {
	opt := optype(data[0])
	switch opt {
	case optypeUserOpened:
		op := &userOpenedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			c.logger.Error("Failed to unmarshal user opened op", xlog.Err(err))
			return
		}
		c.broker.userOpenedOp(op)
	case optypeUserClosed:
		op := &userClosedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			c.logger.Error("Failed to unmarshal user closed op", xlog.Err(err))
			return
		}
		c.broker.userClosedOp(op)
	case optypeJoinChannel:
		op := &joinChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			c.logger.Error("Failed to unmarshal join channel op", xlog.Err(err))
			return
		}
		c.broker.joinChannelOp(op)
	case optypeLeaveChannel:
		op := &leaveChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			c.logger.Error("Failed to unmarshal leave channel op", xlog.Err(err))
			return
		}
		c.broker.leaveChannelOp(op)
	default:
		c.logger.Error("Unknown operation type", xlog.U32("type", uint32(opt)))
		return
	}
}

func (c *cluster) applyRemoveNode(id uint64) {
	c.broker.userClients.Delete(id)
	c.broker.clientChannels.Delete(id)
	c.broker.channelClients.Delete(id)
	if c.peers.Delete(id) {
		c.logger.Info("peer deleted", xlog.Peer(id))
	}
	c.size = c.peers.Len() + 1
	if c.broker.id == id {
		c.leader.Store(0)
		c.ready.Store(false)
		c.broker.ExpelAllConns()
	}
}

func (c *cluster) attemptSnapshot() {
	if c.appliedIndex-c.snapshotIndex <= 1024 {
		return
	}
	data, err := c.broker.makeSnapshot()
	if err != nil {
		panic(err)
	}
	_, err = c.storage.CreateSnapshot(c.appliedIndex, c.confState, data)
	if err != nil {
		panic(err)
	}
	c.snapshotIndex = c.appliedIndex
	compactIndex := uint64(1)
	if c.appliedIndex > 1024 {
		compactIndex = c.appliedIndex - 1024
	}
	if err := c.storage.Compact(compactIndex); err != nil {
		if !errors.Is(err, raft.ErrCompacted) {
			panic(err)
		}
	}
}

func (c *cluster) applySnapshot(snap raftpb.Snapshot) {
	if snap.Metadata.Index <= c.appliedIndex {
		panic(fmt.Sprintf("snapshot index [%d] should > progress.appliedIndex [%d]", snap.Metadata.Index, c.appliedIndex))
	}
	var s snapshot
	if err := json.Unmarshal(snap.Data, &s); err != nil {
		panic(err)
	}
	if c.appliedIndex != 0 {
		c.logger.Warn("override existing state")
	}
	c.broker.recoverFromSnapshot(s)
	c.confState = &snap.Metadata.ConfState
	c.snapshotIndex = snap.Metadata.Index
	c.appliedIndex = snap.Metadata.Index
}
