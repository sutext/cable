package broker

import (
	"context"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/xlog"
)

func (b *broker) addRaftNode(id uint64) {
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeAddNode,
		NodeID: id,
	}
	b.node.ProposeConfChange(context.Background(), cc)
}

func (b *broker) removeRaftNode(id uint64) {
	cc := raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: id,
	}
	b.node.ProposeConfChange(context.Background(), cc)
}

func (b *broker) startRaft(join bool) {
	if b.node != nil {
		return
	}
	storage := raft.NewMemoryStorage()
	b.storage = storage
	c := &raft.Config{
		ID:              b.id,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   4096,
		MaxInflightMsgs: 256,
	}
	if join {
		b.node = raft.RestartNode(c)
	} else {
		initPeers := make([]raft.Peer, 0, b.peers.Len())
		b.peers.Range(func(key uint64, value *peerClient) bool {
			initPeers = append(initPeers, raft.Peer{ID: key})
			return true
		})
		b.node = raft.StartNode(c, initPeers)
	}
	go b.raftLoop()
}

// raftLoop 处理Raft节点的消息循环
func (b *broker) raftLoop() {
	b.lastLeader = time.Now()
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-b.shutdownCh:
			return
		case <-ticker.C:
			b.node.Tick()
		case rd := <-b.node.Ready():
			// 保存HardState和Entries到存储
			if !raft.IsEmptyHardState(rd.HardState) {
				if err := b.storage.SetHardState(rd.HardState); err != nil {
					b.logger.Error("Failed to set hard state", xlog.Err(err))
				}
			}

			if len(rd.Entries) > 0 {
				if err := b.storage.Append(rd.Entries); err != nil {
					b.logger.Error("Failed to append entries", xlog.Err(err))
				}
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				if err := b.storage.ApplySnapshot(rd.Snapshot); err != nil {
					b.logger.Error("Failed to apply snapshot", xlog.Err(err))
				}
			}

			// 处理已提交的日志
			for _, entry := range rd.CommittedEntries {
				b.processCommittedEntry(entry)
			}
			// 处理leader变化
			if rd.SoftState != nil {
				newLeader := rd.SoftState.Lead
				if b.leaderID.Load() != newLeader {
					b.leaderID.Store(newLeader)
					b.isLeader.Store(b.id == newLeader)
					b.logger.Info("New leader elected", xlog.Peer(newLeader))
				}
			}
			// 发送Raft消息给其他节点
			for _, msg := range rd.Messages {
				// 根据消息类型和目标节点处理
				if peer, ok := b.peers.Get(msg.To); ok {
					// 使用现有的peerClient发送Raft消息
					if err := peer.sendRaftMessage(context.Background(), msg); err != nil {
						b.logger.Error("Failed to send raft message to peer", xlog.Err(err))
					}
				}
				// 记录leader活动
				if msg.Type == raftpb.MsgHeartbeat {
					b.lastLeader = time.Now()
				}
			}
			// 推进Raft节点状态
			b.node.Advance()
		}
	}
}

// processCommittedEntry 处理已提交的Raft日志条目
func (b *broker) processCommittedEntry(entry raftpb.Entry) {
	// 处理不同类型的日志条目
	switch entry.Type {
	case raftpb.EntryNormal:
		if len(entry.Data) > 0 {
			// 应用日志到状态机
			b.applyLogToStateMachine(entry.Data)
		}
	case raftpb.EntryConfChange:
		// 处理配置变更
		var cc raftpb.ConfChange
		if err := cc.Unmarshal(entry.Data); err != nil {
			b.logger.Error("Failed to unmarshal conf change", xlog.Err(err))
			return
		}
		b.node.ApplyConfChange(cc)
		// 更新本地节点列表
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

// applyLogToStateMachine 应用日志到状态机
func (b *broker) applyLogToStateMachine(data []byte) {
	opt := optype(data[0])
	switch opt {
	case optypeUserOpened:
		op := &userOpenedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal user opened op", xlog.Err(err))
			return
		}
		b.handleUserOpeed(op)
	case optypeUserClosed:
		op := &userClosedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal user closed op", xlog.Err(err))
			return
		}
		b.handleUserClosed(op)
	case optypeJoinChannel:
		op := &joinChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal join channel op", xlog.Err(err))
			return
		}
		b.handleJoinChannel(op)
	case optypeLeaveChannel:
		op := &leaveChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal leave channel op", xlog.Err(err))
			return
		}
		b.handleLeaveChannel(op)
	default:
		b.logger.Error("Unknown operation type", xlog.U32("type", uint32(opt)))
		return
	}
}

func (b *broker) handleUserOpeed(op *userOpenedOp) {
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

func (b *broker) handleUserClosed(op *userClosedOp) {
	b.userClients.DeleteKey(op.uid, op.cid)
	b.clientChannels.RangeKey(op.cid, func(channel string, _ struct{}) bool {
		b.channelClients.DeleteKey(channel, op.cid)
		return true
	})
	b.clientChannels.Delete(op.cid)
}

func (b *broker) handleJoinChannel(op *joinChannelOp) {
	b.userClients.RangeKey(op.uid, func(cid string, idnet idAndNet) bool {
		for _, ch := range op.channels {
			b.channelClients.SetKey(ch, cid, idnet)
			b.clientChannels.SetKey(cid, ch, struct{}{})
		}
		return true
	})
}
func (b *broker) handleLeaveChannel(op *leaveChannelOp) {
	b.userClients.RangeKey(op.uid, func(cid string, idnet idAndNet) bool {
		for _, ch := range op.channels {
			b.channelClients.DeleteKey(ch, cid)
			b.clientChannels.DeleteKey(cid, ch)
		}
		return true
	})
}

// SubmitOperation 提交状态机操作到Raft集群
func (b *broker) SubmitOperation(op opdata) error {
	enc := coder.NewEncoder()
	enc.WriteUInt8(uint8(op.opt()))
	op.WriteTo(enc)
	b.node.Propose(context.Background(), enc.Bytes())
	return nil
}

// GetStateMachineStatus 获取状态机状态
func (b *broker) GetStateMachineStatus() map[string]interface{} {
	// 返回状态机状态信息
	return map[string]interface{}{
		"user_count":    b.userClients.Len(),
		"channel_count": b.channelClients.Len(),
		"broker_id":     b.id,
	}
}
