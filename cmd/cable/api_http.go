package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

type httpServer struct {
	hs     *http.Server
	mux    *http.ServeMux
	booter *booter
	broker cluster.Broker
}

func newHTTP(booter *booter) *httpServer {
	s := &httpServer{
		mux:    http.NewServeMux(),
		broker: booter.broker,
		booter: booter,
	}
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/inspect", s.handleInspect)
	s.mux.HandleFunc("/isOnline", s.handleIsOnline)
	s.mux.HandleFunc("/kickUser", s.handleKickUser)
	s.mux.HandleFunc("/kickNode", s.handleKickNode)
	s.mux.HandleFunc("/sendToAll", s.handleSendToAll)
	s.mux.HandleFunc("/sendToUser", s.handleSendToUser)
	s.mux.HandleFunc("/sendToChannel", s.handleSendToChannel)
	s.mux.HandleFunc("/joinChannel", s.handleJoinChannel)
	s.mux.HandleFunc("/leaveChannel", s.handleLeaveChannel)
	s.mux.HandleFunc("/listChannels", s.handleListChannels)
	s.mux.HandleFunc("/listUsers", s.handleListUsers)
	s.hs = &http.Server{Addr: fmt.Sprintf(":%d", booter.config.HTTPPort), Handler: s.mux}
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
func (s *httpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
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
func (s *httpServer) handleIsOnline(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	online := s.broker.IsOnline(r.Context(), uid)
	data, err := json.MarshalIndent(map[string]bool{"online": online}, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *httpServer) handleKickUser(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	s.broker.KickUser(r.Context(), uid)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
func (s *httpServer) handleKickNode(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kickReq := &struct {
		ID uint64 `json:"id"`
	}{}
	err := json.NewDecoder(r.Body).Decode(kickReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if kickReq.ID == 0 {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	err = s.broker.Cluster().KickBroker(r.Context(), kickReq.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

type msgResult struct {
	Total   int32 `json:"total"`
	Success int32 `json:"success"`
}

func (s *httpServer) handleSendToAll(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	msg, err := s.parseMessage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	total, success, err := s.broker.SendToAll(r.Context(), msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := msgResult{
		Total:   total,
		Success: success,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func (s *httpServer) handleSendToUser(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid := r.Header.Get("message-uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	msg, err := s.parseMessage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	total, success, err := s.broker.SendToUser(r.Context(), uid, msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := msgResult{
		Total:   total,
		Success: success,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func (s *httpServer) handleSendToChannel(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	channel := r.Header.Get("message-channel")
	if channel == "" {
		http.Error(w, "channel is required", http.StatusBadRequest)
		return
	}
	msg, err := s.parseMessage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	total, success, err := s.broker.SendToChannel(r.Context(), channel, msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := msgResult{
		Total:   total,
		Success: success,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

type JoinReq struct {
	UID      string            `json:"uid"`
	Channels map[string]string `json:"channels"`
}

func (s *httpServer) handleJoinChannel(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &JoinReq{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	if len(req.Channels) == 0 {
		http.Error(w, "channels is required", http.StatusBadRequest)
		return
	}
	err = s.broker.JoinChannel(r.Context(), req.UID, req.Channels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
func (s *httpServer) handleLeaveChannel(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &JoinReq{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	if len(req.Channels) == 0 {
		http.Error(w, "channels is required", http.StatusBadRequest)
		return
	}
	err = s.broker.LeaveChannel(r.Context(), req.UID, req.Channels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
func (s *httpServer) handleListChannels(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	channels, err := s.booter.ListChannels(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(channels, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
func (s *httpServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !s.broker.Cluster().IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	brokerID, err := strconv.ParseUint(r.URL.Query().Get("brokerID"), 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	users, err := s.broker.ListUsers(r.Context(), brokerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *httpServer) parseMessage(r *http.Request) (*packet.Message, error) {
	qos, err := packet.ParseQos(r.Header.Get("message-qos"))
	if err != nil {
		return nil, err
	}
	kind, err := packet.ParseKind(r.Header.Get("message-kind"))
	if err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return &packet.Message{
		Qos:     qos,
		Kind:    kind,
		Payload: payload,
	}, nil
}
