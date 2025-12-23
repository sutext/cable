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

func merge(i, o *protos.Inspects) {
	i.Id = "all"
	i.UserCount += o.UserCount
	i.ClientCount += o.ClientCount
	i.ClusterSize = max(i.ClusterSize, o.ClusterSize)
	i.ChannelCount += o.ChannelCount
}

func (b *broker) inspect() *protos.Inspects {
	// peersInpsects := make(map[string]*protos.PeerInspect)
	// b.peers.Range(func(k string, v *peer_client) bool {
	// 	peersInpsects[k] = &protos.PeerInspect{
	// 		Status:      v.client.Status().String(),
	// 		SendRate:    v.client.SendRate(),
	// 		WriteRate:   v.client.WriteRate(),
	// 		QueueLength: v.client.SendQueueLength(),
	// 	}
	// 	return true
	// })
	return &protos.Inspects{
		Id:           b.id,
		UserCount:    b.userClients.Len(),
		ClientCount:  b.clientChannels.Len(),
		ClusterSize:  b.clusterSize(),
		ChannelCount: b.clientChannels.Len(),
	}
}

func (b *broker) Inspects(ctx context.Context) ([]*protos.Inspects, error) {
	inspects := make([]*protos.Inspects, 2)
	inspects[0] = &protos.Inspects{}
	inspects[1] = b.inspect()
	merge(inspects[0], inspects[1])
	wg := sync.WaitGroup{}
	b.peers.Range(func(k string, v *peerClient) bool {
		wg.Go(func() {
			isp, err := v.inspect(ctx)
			if err != nil {
				b.logger.Error("inspect peer failed", xlog.Peer(v.id), xlog.Err(err))
			}
			merge(inspects[0], isp)
			inspects = append(inspects, isp)
		})
		return true
	})
	wg.Wait()
	return inspects, nil
}

func (b *broker) handleInspect(w http.ResponseWriter, r *http.Request) {
	isps, err := b.Inspects(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(isps, "", "  ")
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
	count, err := b.JoinChannel(r.Context(), uid, channels...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]int32{
		"count": count,
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
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
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
