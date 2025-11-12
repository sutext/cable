package broker

import (
	"encoding/json"
	"net/http"

	"sutext.github.io/cable/packet"
)

type Inspect struct {
	ID       string         `json:"id"`
	Clients  int            `json:"clients"`
	Channels map[string]int `json:"channels"`
}

func NewInspect() *Inspect {
	return &Inspect{
		Channels: make(map[string]int),
	}
}
func (i *Inspect) merge(o *Inspect) {
	i.ID = "all"
	i.Clients += o.Clients
	for k, v := range o.Channels {
		if _, ok := i.Channels[k]; !ok {
			i.Channels[k] = 0
		}
		i.Channels[k] += v
	}
}
func (b *broker) inspect() *Inspect {
	return &Inspect{
		ID:       b.id,
		Clients:  len(b.users.Dump()),
		Channels: b.channels.Dump(),
	}
}
func (b *broker) handleInspect(w http.ResponseWriter, r *http.Request) {
	isps, err := b.Inspects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(isps, "", "\t")
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
	data, err := json.MarshalIndent(resp, "", "\t")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
