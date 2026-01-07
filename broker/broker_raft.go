package broker

import (
	"context"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/xlog"
)

func (b *broker) compactRaftStorage() {
	idx, err := b.raftStorage.LastIndex()
	if err != nil {
		b.logger.Error("Failed to get last index", xlog.Err(err))
		return
	}
	b.raftStorage.Compact(idx)
}
func (b *broker) startRaft(join bool) {
	if b.raftNode != nil {
		return
	}
	storage := raft.NewMemoryStorage()
	b.raftStorage = storage
	c := &raft.Config{
		ID:              b.id,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   4096,
		MaxInflightMsgs: 256,
		Logger:          nil,
	}
	if join {
		b.raftNode = raft.RestartNode(c)
	} else {
		initPeers := make([]raft.Peer, b.clusterSize)
		initPeers[0] = raft.Peer{ID: b.id}
		b.peers.Range(func(key uint64, value *peerClient) bool {
			initPeers = append(initPeers, raft.Peer{ID: key})
			return true
		})
		b.raftNode = raft.StartNode(c, initPeers)
	}
	go b.raftLoop()
}

func (b *broker) raftLoop() {
	b.lastLeader = time.Now()
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopRaft:
			return
		case <-ticker.C:
			b.raftNode.Tick()
		case rd := <-b.raftNode.Ready():
			if !raft.IsEmptyHardState(rd.HardState) {
				if err := b.raftStorage.SetHardState(rd.HardState); err != nil {
					b.logger.Error("Failed to set hard state", xlog.Err(err))
				}
			}
			if len(rd.Entries) > 0 {
				if err := b.raftStorage.Append(rd.Entries); err != nil {
					b.logger.Error("Failed to append entries", xlog.Err(err))
				}
			}
			if !raft.IsEmptySnap(rd.Snapshot) {
				if err := b.raftStorage.ApplySnapshot(rd.Snapshot); err != nil {
					b.logger.Error("Failed to apply snapshot", xlog.Err(err))
				}
				if err := b.applySnapshot(rd.Snapshot); err != nil {
					b.logger.Error("Failed to apply snapshot", xlog.Err(err))
				}
			}
			for _, entry := range rd.CommittedEntries {
				b.processRaftEntry(entry)
			}
			if rd.SoftState != nil {
				newLeader := rd.SoftState.Lead
				if b.raftLeader.Load() != newLeader {
					b.raftLeader.Store(newLeader)
					b.isLeader.Store(b.id == newLeader)
					b.logger.Info("New leader elected", xlog.U64("leader", newLeader))
				}
				b.ready.Store(true)
			}
			for _, msg := range rd.Messages {
				if peer, ok := b.peers.Get(msg.To); ok {
					if err := peer.sendRaftMessage(context.Background(), msg); err != nil {
						b.logger.Error("Failed to send raft message to peer", xlog.Err(err))
					}
				}
				if msg.Type == raftpb.MsgHeartbeat {
					b.lastLeader = time.Now()
				}
			}
			b.raftNode.Advance()
		}
	}
}

func (b *broker) processRaftEntry(entry raftpb.Entry) {
	switch entry.Type {
	case raftpb.EntryNormal:
		if len(entry.Data) > 0 {
			b.processRaftData(entry.Data)
		}
	case raftpb.EntryConfChange, raftpb.EntryConfChangeV2:
		var cc raftpb.ConfChange
		if err := cc.Unmarshal(entry.Data); err != nil {
			b.logger.Error("Failed to unmarshal conf change", xlog.Err(err))
			return
		}
		b.raftNode.ApplyConfChange(cc)
		switch cc.Type {
		case raftpb.ConfChangeAddNode:
			b.clusterSize = b.peers.Len() + 1
		case raftpb.ConfChangeRemoveNode:
			b.removePeer(cc.NodeID)
		}
	}
}

func (b *broker) processRaftData(data []byte) {
	opt := optype(data[0])
	switch opt {
	case optypeUserOpened:
		op := &userOpenedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal user opened op", xlog.Err(err))
			return
		}
		b.raftUserOpened(op)
	case optypeUserClosed:
		op := &userClosedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal user closed op", xlog.Err(err))
			return
		}
		b.raftUserClosed(op)
	case optypeJoinChannel:
		op := &joinChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal join channel op", xlog.Err(err))
			return
		}
		b.raftJoinChannel(op)
	case optypeLeaveChannel:
		op := &leaveChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal leave channel op", xlog.Err(err))
			return
		}
		b.raftLeaveChannel(op)
	default:
		b.logger.Error("Unknown operation type", xlog.U32("type", uint32(opt)))
		return
	}
}
func (b *broker) removePeer(id uint64) {
	b.raftLock.Lock()
	if userClients, ok := b.userClients.Get(id); ok {
		userClients.Range(func(uid string, m *safe.Map[string, string]) bool {
			m.Range(func(cid string, net string) bool {
				b.clientChannels.Delete(cid)
				return true
			})
			return true
		})
		b.userClients.Delete(id)
	}
	b.channelClients.Delete(id)
	b.raftLock.Unlock()
	if b.peers.Delete(id) {
		b.logger.Info("peer deleted", xlog.Peer(id))
	}
}

func (b *broker) raftUserOpened(op *userOpenedOp) {
	b.raftLock.Lock()
	defer b.raftLock.Unlock()
	userClients, _ := b.userClients.GetOrSet(op.brokerID, &safe.KeyMap[string]{})
	userClients.SetKey(op.uid, op.cid, op.net)
	channelClients, _ := b.channelClients.GetOrSet(op.brokerID, &safe.KeyMap[string]{})
	if chs, err := b.handler.GetChannels(op.uid); err == nil {
		for _, ch := range chs {
			b.clientChannels.SetKey(op.cid, ch, struct{}{})
			channelClients.SetKey(ch, op.cid, op.net)
		}
	}
}

func (b *broker) raftUserClosed(op *userClosedOp) {
	b.raftLock.Lock()
	defer b.raftLock.Unlock()
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		value.DeleteKey(op.uid, op.cid)
		return true
	})
	b.clientChannels.RangeKey(op.cid, func(channel string, _ struct{}) bool {
		b.channelClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
			value.DeleteKey(channel, op.cid)
			return true
		})
		return true
	})
	b.clientChannels.Delete(op.cid)
}

func (b *broker) raftJoinChannel(op *joinChannelOp) {
	b.raftLock.Lock()
	defer b.raftLock.Unlock()
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		value.RangeKey(op.uid, func(cid string, net string) bool {
			for _, ch := range op.channels {
				channelClients.SetKey(ch, cid, net)
				b.clientChannels.SetKey(cid, ch, struct{}{})
			}
			return true
		})
		return true
	})
}
func (b *broker) raftLeaveChannel(op *leaveChannelOp) {
	b.raftLock.Lock()
	defer b.raftLock.Unlock()
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		value.RangeKey(op.uid, func(cid string, net string) bool {
			for _, ch := range op.channels {
				channelClients.DeleteKey(ch, cid)
				b.clientChannels.DeleteKey(cid, ch)
			}
			return true
		})
		return true
	})
}

func (b *broker) submitRaftOp(ctx context.Context, op opdata) error {
	enc := coder.NewEncoder()
	enc.WriteUInt8(uint8(op.opt()))
	op.WriteTo(enc)
	b.raftNode.Propose(ctx, enc.Bytes())
	return nil
}
func (b *broker) addRaftNode(ctx context.Context, id uint64) error {
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeAddNode,
		NodeID: id,
	}
	return b.raftNode.ProposeConfChange(ctx, cc)
}

func (b *broker) removeRaftNode(ctx context.Context, id uint64) error {
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: id,
	}
	return b.raftNode.ProposeConfChange(ctx, cc)
}
