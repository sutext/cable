package broker

import (
	"context"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/xlog"
)

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
			}
			for _, entry := range rd.CommittedEntries {
				b.processRaftEntry(entry)
			}
			if rd.SoftState != nil {
				newLeader := rd.SoftState.Lead
				if b.raftLeader.Load() != newLeader {
					b.raftLeader.Store(newLeader)
					b.isLeader.Store(b.id == newLeader)
					b.logger.Info("New leader elected", xlog.Peer(newLeader))
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
			if b.peers.Delete(cc.NodeID) {
				b.logger.Info("peer deleted", xlog.Peer(cc.NodeID))
			}
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

func (b *broker) raftUserOpened(op *userOpenedOp) {
	b.userClients.SetKey(op.uid, op.cid, idAndNet{
		id:  op.brokerID,
		net: op.net,
	})
	if chs, err := b.handler.GetChannels(op.uid); err == nil {
		for _, ch := range chs {
			b.clientChannels.SetKey(op.cid, ch, struct{}{})
			b.channelClients.SetKey(ch, op.cid, idAndNet{id: op.brokerID, net: op.net})
		}
	}
}

func (b *broker) raftUserClosed(op *userClosedOp) {
	b.userClients.DeleteKey(op.uid, op.cid)
	b.clientChannels.RangeKey(op.cid, func(channel string, _ struct{}) bool {
		b.channelClients.DeleteKey(channel, op.cid)
		return true
	})
	b.clientChannels.Delete(op.cid)
}

func (b *broker) raftJoinChannel(op *joinChannelOp) {
	b.userClients.RangeKey(op.uid, func(cid string, idnet idAndNet) bool {
		for _, ch := range op.channels {
			b.channelClients.SetKey(ch, cid, idnet)
			b.clientChannels.SetKey(cid, ch, struct{}{})
		}
		return true
	})
}
func (b *broker) raftLeaveChannel(op *leaveChannelOp) {
	b.userClients.RangeKey(op.uid, func(cid string, idnet idAndNet) bool {
		for _, ch := range op.channels {
			b.channelClients.DeleteKey(ch, cid)
			b.clientChannels.DeleteKey(cid, ch)
		}
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
func (b *broker) addRaftNode(ctx context.Context, id uint64) {
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeAddNode,
		NodeID: id,
	}
	b.raftNode.ProposeConfChange(ctx, cc)
}

func (b *broker) removeRaftNode(ctx context.Context, id uint64) {
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: id,
	}
	b.raftNode.ProposeConfChange(ctx, cc)
}
