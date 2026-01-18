package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

type httpServer struct {
	mux    *http.ServeMux
	broker cluster.Broker
	hs     *http.Server
}

func newHTTP(broker cluster.Broker, port uint16) *httpServer {
	s := &httpServer{
		mux:    http.NewServeMux(),
		broker: broker,
	}
	s.mux.HandleFunc("/join", s.handleJoin)
	s.mux.HandleFunc("/send", s.handleSendMessage)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/inspect", s.handleInspect)
	s.mux.HandleFunc("/kickUser", s.handleKickUser)
	s.mux.HandleFunc("/kickNode", s.handleKickNode)
	s.hs = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: s.mux}
	return s

}

func (s *httpServer) Serve() error {
	return s.hs.ListenAndServe()
}
func (s *httpServer) Shutdown() error {
	return s.hs.Shutdown(context.Background())
}
func (s *httpServer) Handle(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}
func (s *httpServer) handleInspect(w http.ResponseWriter, r *http.Request) {
	ss, err := s.broker.Inspects(r.Context())
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

func (s *httpServer) handleKickUser(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	s.broker.KickUser(r.Context(), uid)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *httpServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
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
		total, success, err = s.broker.SendToUser(r.Context(), uid, msgPacket)
	} else if channel != "" {
		total, success, err = s.broker.SendToChannel(r.Context(), channel, msgPacket)
	} else {
		total, success, err = s.broker.SendToAll(r.Context(), msgPacket)
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

func (s *httpServer) handleJoin(w http.ResponseWriter, r *http.Request) {
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
	strs := strings.Split(chs, ",")
	channels := make(map[string]string)
	for _, ch := range strs {
		channels[ch] = ""
	}
	err := s.broker.JoinChannel(r.Context(), uid, channels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *httpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *httpServer) handleKickNode(w http.ResponseWriter, r *http.Request) {
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
	err = s.broker.Cluster().KickBroker(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
