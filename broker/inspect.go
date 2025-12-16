package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type InspectPeer struct {
	Status      string  `json:"status"`
	QueueLength int     `json:"queue_length"`
	SendRate    float64 `json:"send_rate"`
	WriteRate   float64 `json:"write_rate"`
}
type Inspect struct {
	ID          string                  `json:"id"`
	Peers       map[string]*InspectPeer `json:"peers"`
	TopConns    map[string]int          `json:"top_conns"`
	TopPeers    map[string]int          `json:"top_peers"`
	UserCount   int                     `json:"user_count"`
	ClientCount int                     `json:"client_count"`
	ClusterSize int32                   `json:"cluster_size"`
}

func NewInspect() *protos.InspectResp {
	return &protos.InspectResp{}
}

func merge(i, o *protos.InspectResp) {
	i.Id = "all"
	i.UserCount += o.UserCount
	i.ClientCount += o.ClientCount
	i.ClusterSize = max(i.ClusterSize, o.ClusterSize)
}

func (b *broker) inspect() *protos.InspectResp {
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
	users := b.userClients.Len()
	clients := 0
	b.userClients.Range(func(uid string, cs *safe.XMap[string, server.Network]) bool {
		clients += int(cs.Len())
		return true
	})
	l := b.listeners[server.NetworkTCP]
	return &protos.InspectResp{
		Id: b.id,
		// Peers: peersInpsects,
		// TopPeers:    b.peerServer.Top(),
		TopConns:    l.Top(),
		UserCount:   int32(users),
		ClientCount: int32(clients),
		ClusterSize: b.clusterSize(),
	}
}

func (b *broker) Inspects() ([]*protos.InspectResp, error) {
	ctx := context.Background()
	inspects := make([]*protos.InspectResp, 2)
	inspects[0] = &protos.InspectResp{}
	inspects[1] = b.inspect()
	merge(inspects[0], inspects[1])
	b.peers.Range(func(k string, v *peer_client) bool {
		isp, err := v.inspect(ctx)
		if err != nil {
			b.logger.Error("inspect failed", xlog.Peer(k), xlog.Err(err))
			return true
		}
		merge(inspects[0], isp)
		inspects = append(inspects, isp)
		return true
	})
	return inspects, nil
}

func (b *broker) handleInspect(w http.ResponseWriter, r *http.Request) {
	isps, err := b.Inspects()
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
