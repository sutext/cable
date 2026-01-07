package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

func (b *broker) inspect() *protos.Status {
	return &protos.Status{
		Id:           b.id,
		UserCount:    b.userClients.Len(),
		ClientCount:  b.clientChannels.Len(),
		ClusterSize:  b.clusterSize,
		ChannelCount: b.clientChannels.Len(),
		RaftState:    b.raftNode.Status().String(),
	}
}

func (b *broker) Inspects(ctx context.Context) ([]*protos.Status, error) {
	ss := make([]*protos.Status, 0, b.clusterSize)
	ss[0] = b.inspect()
	wg := sync.WaitGroup{}
	b.peers.Range(func(id uint64, cli *peerClient) bool {
		wg.Go(func() {
			s, err := cli.inspect(ctx)
			if err != nil {
				b.logger.Error("inspect peer failed", xlog.Peer(id), xlog.Err(err))
				return
			}
			ss = append(ss, s)
		})
		return true
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
	if b.raftNode == nil {
		http.Error(w, "node is nil", http.StatusInternalServerError)
		return
	}
	if !b.ready.Load() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
