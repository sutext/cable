package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Inspect struct {
	ID            string `json:"id"`
	Peers         int    `json:"peers"`
	Users         int    `json:"users"`
	Clients       int    `json:"clients"`
	ClusterSize   int32  `json:"cluster_size"`
	OnlienUsers   int    `json:"online_users"`
	OnlineClients int    `json:"online_clients"`
}

func NewInspect() *Inspect {
	return &Inspect{}
}

func (i *Inspect) merge(o *Inspect) {
	i.ID = "all"
	i.Peers += o.Peers
	i.Users += o.Users
	i.Clients += o.Clients
	i.ClusterSize = max(i.ClusterSize, o.ClusterSize)
	i.OnlienUsers += o.OnlienUsers
	i.OnlineClients += o.OnlineClients
}

func (b *broker) inspect() *Inspect {
	peers := 0
	b.peers.Range(func(k string, v *peer) bool {
		if v.IsReady() {
			peers++
		}
		return true
	})
	users := 0
	clients := 0
	onlienUsers := 0
	onlineClients := 0
	b.userClients.Range(func(cid string) bool {
		if b.isOnline(cid) {
			onlienUsers++
		}
		oldClients := clients
		b.userClients.RangeKey(cid, func(cid string, net server.Network) bool {
			clients++
			if b.isActive(cid, net) {
				onlineClients++
			}
			return true
		})
		if oldClients != clients {
			users++
		} else {
			b.userClients.Delete(cid)
		}
		return true
	})
	return &Inspect{
		ID:            b.id,
		Peers:         peers,
		Users:         users,
		Clients:       clients,
		ClusterSize:   b.clusterSize(),
		OnlienUsers:   onlienUsers,
		OnlineClients: onlineClients,
	}
}

func (b *broker) Inspects() ([]*Inspect, error) {
	ctx := context.Background()
	inspects := make([]*Inspect, 2)
	inspects[0] = NewInspect()
	inspects[1] = b.inspect()
	inspects[0].merge(inspects[1])
	b.peers.Range(func(k string, v *peer) bool {
		isp, err := v.inspect(ctx)
		if err != nil {
			return true
		}
		inspects[0].merge(isp)
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
	var total, success uint64
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
	resp := map[string]uint64{
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
	resp := map[string]uint64{
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
	resp := map[string]uint64{
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
