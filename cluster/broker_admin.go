package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"sutext.github.io/cable/cluster/pb"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

func (b *broker) inspect() *pb.Status {
	userCount := make(map[uint64]int32)
	b.userClients.Range(func(key uint64, value *safe.KeyMap[string]) bool {
		userCount[key] = value.Len()
		return true
	})
	channelCount := make(map[uint64]int32)
	b.channelClients.Range(func(key uint64, value *safe.KeyMap[string]) bool {
		channelCount[key] = value.Len()
		return true
	})
	status := b.cluster.Status()
	progress := make(map[uint64]*pb.RaftProgress)
	if status.Progress != nil {
		for id, p := range status.Progress {
			progress[id] = &pb.RaftProgress{
				Match: p.Match,
				Next:  p.Next,
				State: p.State.String(),
			}
		}
	}
	return &pb.Status{
		Id:           b.id,
		UserCount:    userCount,
		ClientCount:  b.clientChannels.Len(),
		ClusterSize:  b.cluster.size,
		ChannelCount: channelCount,
		RaftState:    status.RaftState.String(),
		RaftTerm:     status.Term,
		RaftLogSize:  b.cluster.RaftLogSize(),
		RaftApplied:  status.Applied,
		RaftProgress: progress,
	}
}

func (b *broker) Inspects(ctx context.Context) ([]*pb.Status, error) {
	ss := make([]*pb.Status, 0, b.cluster.size)
	ss = append(ss, b.inspect())
	wg := sync.WaitGroup{}
	b.cluster.RangePeers(func(id uint64, cli *peerClient) {
		wg.Go(func() {
			s, err := cli.inspect(ctx)
			if err != nil {
				b.logger.Error("inspect peer failed", xlog.Peer(id), xlog.Err(err))
				return
			}
			ss = append(ss, s)
		})
	})
	wg.Wait()
	return ss, nil
}

func (b *broker) handleInspect(w http.ResponseWriter, r *http.Request) {
	ss, err := b.Inspects(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(ss, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func (b *broker) handleKickout(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	b.KickUser(r.Context(), uid)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
func (b *broker) handleMessage(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	channel := r.URL.Query().Get("channel")
	msg := r.URL.Query().Get("msg")
	if msg == "" {
		http.Error(w, "msg is required", http.StatusBadRequest)
		return
	}
	msgPacket := packet.NewMessage([]byte(msg))
	var total, success int32
	var err error
	if uid != "" {
		total, success, err = b.SendToUser(r.Context(), uid, msgPacket)
	} else if channel != "" {
		total, success, err = b.SendToChannel(r.Context(), channel, msgPacket)
	} else {
		total, success, err = b.SendToAll(r.Context(), msgPacket)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]int32{
		"total":   total,
		"success": success,
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (b *broker) handleJoin(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	chs := r.URL.Query().Get("channels")
	if chs == "" {
		http.Error(w, "channels is required", http.StatusBadRequest)
		return
	}
	channels := strings.Split(chs, ",")
	err := b.JoinChannel(r.Context(), uid, channels...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
func (b *broker) handleBrodcast(w http.ResponseWriter, r *http.Request) {
	msg := r.URL.Query().Get("msg")
	if msg == "" {
		http.Error(w, "msg is required", http.StatusBadRequest)
		return
	}
	total, success, err := b.SendToAll(r.Context(), packet.NewMessage([]byte(msg)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]int32{
		"total":   total,
		"success": success,
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func (b *broker) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !b.cluster.IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
func (b *broker) handleRemoveNode(w http.ResponseWriter, r *http.Request) {
	str := r.URL.Query().Get("nodeId")
	if str == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	err = b.cluster.RemoveBroker(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
