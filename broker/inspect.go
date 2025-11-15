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
	ID            string         `json:"id"`
	Peers         int            `json:"peers"`
	Users         int            `json:"users"`
	Clients       int            `json:"clients"`
	Channels      map[string]int `json:"channels"`
	OnlienUsers   int            `json:"online_users"`
	OnlineClients int            `json:"online_clients"`
}

func NewInspect() *Inspect {
	return &Inspect{
		Channels: make(map[string]int),
	}
}
func (i *Inspect) merge(o *Inspect) {
	i.ID = "all"
	i.Peers += o.Peers
	i.Users += o.Users
	i.Clients += o.Clients
	i.OnlienUsers += o.OnlienUsers
	i.OnlineClients += o.OnlineClients
	for k, v := range o.Channels {
		if _, ok := i.Channels[k]; !ok {
			i.Channels[k] = 0
		}
		i.Channels[k] += v
	}
}
func (b *broker) inspect() *Inspect {
	peers := 0
	for _, p := range b.peers {
		if p.IsReady() {
			peers++
		}
	}
	users := 0
	clients := 0
	onlienUsers := 0
	onlineClients := 0
	b.users.Range(func(cid string) bool {
		if b.isOnline(cid) {
			onlienUsers++
		}
		oldClients := clients
		b.users.RangeKey(cid, func(cid string, net server.Network) bool {
			clients++
			if b.isActive(cid, net) {
				onlineClients++
			}
			return true
		})
		if oldClients != clients {
			users++
		} else {
			b.users.Delete(cid)
		}
		return true
	})
	channels := b.channels.GetCounts()
	for k, v := range channels {
		if v == 0 {
			b.channels.Delete(k)
		}
	}
	return &Inspect{
		ID:            b.id,
		Peers:         peers,
		Users:         users,
		Clients:       clients,
		Channels:      channels,
		OnlienUsers:   onlienUsers,
		OnlineClients: onlineClients,
	}
}
func (b *broker) Inspects() ([]*Inspect, error) {
	ctx := context.Background()
	inspects := make([]*Inspect, len(b.peers)+2)
	inspects[0] = NewInspect()
	inspects[1] = b.inspect()
	inspects[0].merge(inspects[1])
	for i, p := range b.peers {
		isp, err := p.inspect(ctx)
		if err != nil {
			return nil, err
		}
		inspects[0].merge(isp)
		inspects[i+2] = isp
	}
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
	// var msg packet.Message
	// if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		http.Error(w, "channel is required", http.StatusBadRequest)
		return
	}
	payload := r.URL.Query().Get("payload")
	if payload == "" {
		http.Error(w, "payload is required", http.StatusBadRequest)
		return
	}
	msg := packet.NewMessage([]byte(payload))
	msg.Channel = channel
	total, success, err := b.SendMessage(r.Context(), msg)
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
