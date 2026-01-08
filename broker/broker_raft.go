package broker

import (
	"context"
	"encoding/json"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/xlog"
)

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
	uid string
	cid string
}

func (op *userClosedOp) opt() optype {
	return optypeUserClosed
}
func (op *userClosedOp) WriteTo(e coder.Encoder) error {
	e.WriteString(op.uid)
	e.WriteString(op.cid)
	return nil
}
func (op *userClosedOp) ReadFrom(d coder.Decoder) error {
	var err error
	op.uid, err = d.ReadString()
	if err != nil {
		return err
	}
	op.cid, err = d.ReadString()
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
func (b *broker) startRaft(join bool) {
	if b.raftNode != nil {
		return
	}
	storage := raft.NewMemoryStorage()
	b.raftStorage = storage
	c := &raft.Config{
		ID:                        b.id,
		ElectionTick:              10,
		HeartbeatTick:             1,
		Storage:                   storage,
		MaxSizePerMsg:             1024 * 1024,
		MaxInflightMsgs:           256,
		MaxUncommittedEntriesSize: 1 << 30,
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
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-b.raftStop:
			return
		case <-ticker.C:
			b.raftNode.Tick()
		case rd := <-b.raftNode.Ready():
			if !raft.IsEmptyHardState(rd.HardState) {
				if err := b.raftStorage.SetHardState(rd.HardState); err != nil {
					b.logger.Error("Failed to set hard state", xlog.Err(err))
				}
			}
			if !raft.IsEmptySnap(rd.Snapshot) {
				if err := b.raftStorage.ApplySnapshot(rd.Snapshot); err != nil {
					b.logger.Error("Failed to apply snapshot", xlog.Err(err))
				}
				b.applyRaftSnapshot(rd.Snapshot)
			}
			if len(rd.Entries) > 0 {
				if err := b.raftStorage.Append(rd.Entries); err != nil {
					b.logger.Error("Failed to append entries", xlog.Err(err))
				}
			}
			if len(rd.CommittedEntries) > 0 {
				b.applyRafttEntries(b.entriesToApply(rd.CommittedEntries))
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
					//TODO
				}
			}
			// b.attemptSnapshot()
			b.raftNode.Advance()
		}
	}
}
func (b *broker) entriesToApply(ents []raftpb.Entry) (nents []raftpb.Entry) {
	if len(ents) == 0 {
		return ents
	}
	firstIdx := ents[0].Index
	if firstIdx > b.appliedIndex+1 {
		panic("raft log entries are missing")
	}
	if b.appliedIndex-firstIdx+1 < uint64(len(ents)) {
		nents = ents[b.appliedIndex-firstIdx+1:]
	}
	return nents
}
func (b *broker) applyRafttEntries(entries []raftpb.Entry) {
	for _, entry := range entries {
		switch entry.Type {
		case raftpb.EntryNormal:
			if len(entry.Data) > 0 {
				b.applyRaftOp(entry.Data)
			}
		case raftpb.EntryConfChange, raftpb.EntryConfChangeV2:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(entry.Data); err != nil {
				b.logger.Error("Failed to unmarshal conf change", xlog.Err(err))
				return
			}
			b.confState = b.raftNode.ApplyConfChange(cc)
			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				b.clusterSize = b.peers.Len() + 1
			case raftpb.ConfChangeRemoveNode:
				b.applyRemoveNode(cc.NodeID)
			}
		}
	}
	b.appliedIndex = entries[len(entries)-1].Index
}

func (b *broker) applyRaftOp(data []byte) {
	opt := optype(data[0])
	switch opt {
	case optypeUserOpened:
		op := &userOpenedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal user opened op", xlog.Err(err))
			return
		}
		b.userOpenedOp(op)
	case optypeUserClosed:
		op := &userClosedOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal user closed op", xlog.Err(err))
			return
		}
		b.userClosedOp(op)
	case optypeJoinChannel:
		op := &joinChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal join channel op", xlog.Err(err))
			return
		}
		b.joinChannelOp(op)
	case optypeLeaveChannel:
		op := &leaveChannelOp{}
		if err := coder.Unmarshal(data[1:], op); err != nil {
			b.logger.Error("Failed to unmarshal leave channel op", xlog.Err(err))
			return
		}
		b.leaveChannelOp(op)
	default:
		b.logger.Error("Unknown operation type", xlog.U32("type", uint32(opt)))
		return
	}
}
func (b *broker) userOpenedOp(op *userOpenedOp) {
	userClients, _ := b.userClients.GetOrSet(op.nodeID, &safe.KeyMap[string]{})
	userClients.SetKey(op.uid, op.cid, op.net)
	channelClients, _ := b.channelClients.GetOrSet(op.nodeID, &safe.KeyMap[string]{})
	for _, ch := range op.channels {
		b.clientChannels.SetKey(op.cid, ch, struct{}{})
		channelClients.SetKey(ch, op.cid, op.net)
	}
}

func (b *broker) userClosedOp(op *userClosedOp) {
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

func (b *broker) joinChannelOp(op *joinChannelOp) {
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
func (b *broker) leaveChannelOp(op *leaveChannelOp) {
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
func (b *broker) applyRemoveNode(id uint64) {
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
	if b.peers.Delete(id) {
		b.logger.Info("peer deleted", xlog.Peer(id))
	}
}

type snapshot struct {
	UserClients    map[uint64]map[string]map[string]string `json:"user_clients"`
	ChannelClients map[uint64]map[string]map[string]string `json:"channel_clients"`
	ClientChannels map[string]map[string]struct{}          `json:"client_channels"`
}

func (b *broker) attemptSnapshot() {
	if b.appliedIndex-b.snapshotIndex <= 1024 {
		return
	}
	data, err := b.snapshotData()
	if err != nil {
		panic(err)
	}
	_, err = b.raftStorage.CreateSnapshot(b.appliedIndex, b.confState, data)
	if err != nil {
		panic(err)
	}
	b.snapshotIndex = b.appliedIndex
	// compactIndex := uint64(1)
	// if b.appliedIndex > 1024 {
	// 	compactIndex = b.appliedIndex - 1024
	// }
	// if err := b.raftStorage.Compact(compactIndex); err != nil {
	// 	if !errors.Is(err, raft.ErrCompacted) {
	// 		panic(err)
	// 	}
	// }
}

func (b *broker) snapshotData() ([]byte, error) {
	userClients := make(map[uint64]map[string]map[string]string)
	b.userClients.Range(func(id uint64, v *safe.KeyMap[string]) bool {
		ucmap, ok := userClients[id]
		if !ok {
			ucmap = make(map[string]map[string]string)
			userClients[id] = ucmap
		}
		v.Range(func(uid string, cids *safe.Map[string, string]) bool {
			ucmap[uid] = cids.Load()
			return true
		})
		return true
	})
	channelClients := make(map[uint64]map[string]map[string]string)
	b.channelClients.Range(func(id uint64, v *safe.KeyMap[string]) bool {
		ccmap, ok := channelClients[id]
		if !ok {
			ccmap = make(map[string]map[string]string)
			channelClients[id] = ccmap
		}
		v.Range(func(cid string, uids *safe.Map[string, string]) bool {
			ccmap[cid] = uids.Load()
			return true
		})
		return true
	})
	clientChannels := make(map[string]map[string]struct{})
	b.clientChannels.Range(func(cid string, v *safe.Map[string, struct{}]) bool {
		clientChannels[cid] = v.Load()
		return true
	})
	snapshot := snapshot{
		UserClients:    userClients,
		ChannelClients: channelClients,
		ClientChannels: clientChannels,
	}
	return json.Marshal(snapshot)
}
func (b *broker) applyRaftSnapshot(snap raftpb.Snapshot) {
	if raft.IsEmptySnap(snap) {
		return
	}
	b.logger.Info("Publishing snapshot", xlog.U64("index", snap.Metadata.Index))
	if snap.Metadata.Index <= b.appliedIndex {
		b.logger.Info("Snapshot is older than applied index, skipping")
		return
	}
	var s snapshot
	if err := json.Unmarshal(snap.Data, &s); err != nil {
		b.logger.Error("Failed to unmarshal snapshot", xlog.Err(err))
		return
	}
	for id, ucmap := range s.UserClients {
		userClients, _ := b.userClients.GetOrSet(id, &safe.KeyMap[string]{})
		for uid, cids := range ucmap {
			userClients.Set(uid, safe.NewMap(cids))
		}
	}
	for id, ccmap := range s.ChannelClients {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		for cid, uids := range ccmap {
			channelClients.Set(cid, safe.NewMap(uids))
		}
	}
	for cid, ccs := range s.ClientChannels {
		b.clientChannels.Set(cid, safe.NewMap(ccs))
	}
	b.confState = &snap.Metadata.ConfState
	b.snapshotIndex = snap.Metadata.Index
	b.appliedIndex = snap.Metadata.Index
}
