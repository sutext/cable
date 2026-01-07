package broker

import (
	"encoding/json"
	"errors"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/internal/safe"
)

type snapshot struct {
	UserClients    map[uint64]map[string]map[string]string `json:"user_clients"`
	ChannelClients map[uint64]map[string]map[string]string `json:"channel_clients"`
	ClientChannels map[string]map[string]struct{}          `json:"client_channels"`
}

func (b *broker) createSnapshot() error {
	b.raftLock.Lock()
	defer b.raftLock.Unlock()
	if b.raftNode == nil {
		return errors.New("raft node not initialized")
	}
	status := b.raftNode.Status()
	if status.SoftState.RaftState != raft.StateLeader {
		return errors.New("not leader")
	}
	b.logger.Info("Creating snapshot")
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

	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	b.raftStorage.CreateSnapshot(0, nil, data)
	return nil
}
func (b *broker) applySnapshot(ss raftpb.Snapshot) error {
	b.raftLock.Lock()
	defer b.raftLock.Unlock()
	if b.raftNode == nil {
		return errors.New("raft node not initialized")
	}
	lastIndex, err := b.raftStorage.LastIndex()
	if err != nil {
		return err
	}
	if ss.Metadata.Index <= lastIndex {
		b.logger.Info("Snapshot is already applied")
		return nil
	}
	b.logger.Info("Applying snapshot")
	var s snapshot
	if err := json.Unmarshal(ss.Data, &s); err != nil {
		return err
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
	return nil
}
