//go:generate swag init -g api_http.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	_ "github.com/swaggo/files/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

// @title Cable HTTP API
// @version 1.0
// @description Cable is a high-performance, distributed real-time communication platform.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.example.com/support
// @contact.email support@example.com

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /
// @schemes http

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

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

	// Serve swagger.json and swagger.yaml directly
	s.mux.HandleFunc("/swagger/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./docs/swagger.json")
	})
	s.mux.HandleFunc("/swagger/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./docs/swagger.yaml")
	})

	// Add a simple Swagger UI HTML page that loads swagger.json directly
	s.mux.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html>
		<head>
			<title>Swagger UI</title>
			<link rel="stylesheet" type="text/css" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
		</head>
		<body>
			<div id="swagger-ui"></div>
			<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
			<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-standalone-preset.js"></script>
			<script>
				const ui = SwaggerUIBundle({
					url: "/swagger/swagger.json",
					dom_id: '#swagger-ui',
				
				deepLinking: true,
					docExpansion: 'list',
					presets: [
						SwaggerUIBundle.presets.apis,
						SwaggerUIStandalonePreset
					],
					layout: "StandaloneLayout"
				});
			</script>
		</body>
		</html>`)
	})

	// Redirect /swagger/index.html to /swagger
	s.mux.HandleFunc("/swagger/index.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger", http.StatusMovedPermanently)
	})
	var handler http.Handler = s.mux
	if s.booter.config.Metrics.Enabled || s.booter.config.Trace.Enabled {
		handler = otelhttp.NewHandler(s.mux, "http-api")
	}
	s.hs = &http.Server{Addr: fmt.Sprintf(":%d", booter.config.HTTPPort), Handler: handler}
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

// @Summary Kick node
// @Description Kick a specific node from the cluster
// @Tags cluster
// @Accept json
// @Produce json
// @Param body body map[string]uint64 true "{\"id\": 2}"
// @Success 200 {string} string "ok"
// @Failure 400 {string} string "Invalid request"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /kickNode [post]
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

// @Summary Send message to all users
// @Description Send a message to all connected users
// @Tags message
// @Accept json
// @Produce json
// @Param message-qos header string true "Message QoS level"
// @Param message-kind header string true "Message kind"
// @Param payload body bytes true "Message payload"
// @Success 200 {object} msgResult "{\"total\": 100, \"success\": 98}"
// @Failure 400 {string} string "Invalid request"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /sendToAll [post]
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

// @Summary Send message to user
// @Description Send a message to a specific user
// @Tags message
// @Accept json
// @Produce json
// @Param message-uid header string true "User ID"
// @Param message-qos header string true "Message QoS level"
// @Param message-kind header string true "Message kind"
// @Param payload body bytes true "Message payload"
// @Success 200 {object} msgResult "{\"total\": 1, \"success\": 1}"
// @Failure 400 {string} string "Invalid request"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /sendToUser [post]
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

// @Summary Send message to channel
// @Description Send a message to a specific channel
// @Tags message
// @Accept json
// @Produce json
// @Param message-channel header string true "Channel name"
// @Param message-qos header string true "Message QoS level"
// @Param message-kind header string true "Message kind"
// @Param payload body bytes true "Message payload"
// @Success 200 {object} msgResult "{\"total\": 50, \"success\": 48}"
// @Failure 400 {string} string "Invalid request"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /sendToChannel [post]
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

// @Summary Join channel
// @Description Join a user to one or more channels
// @Tags channel
// @Accept json
// @Produce json
// @Param body body JoinReq true "{\"uid\": \"user1\", \"channels\": {\"channel1\": \"role1\"}}"
// @Success 200 {string} string "ok"
// @Failure 400 {string} string "Invalid request"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /joinChannel [post]
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

// @Summary Leave channel
// @Description Remove a user from one or more channels
// @Tags channel
// @Accept json
// @Produce json
// @Param body body JoinReq true "{\"uid\": \"user1\", \"channels\": {\"channel1\": \"\"}}"
// @Success 200 {string} string "ok"
// @Failure 400 {string} string "Invalid request"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /leaveChannel [post]
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

// @Summary List channels
// @Description List all channels a user has joined
// @Tags channel
// @Accept json
// @Produce json
// @Param uid query string true "User ID"
// @Success 200 {object} map[string]string "{\"channel1\": \"role1\"}"
// @Failure 400 {string} string "uid is required"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /listChannels [get]
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

// @Summary List users
// @Description List all users connected to a specific broker
// @Tags user
// @Accept json
// @Produce json
// @Param brokerID query string true "Broker ID"
// @Success 200 {array} string "[\"user1\", \"user2\"]"
// @Failure 400 {string} string "Invalid brokerID"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "Internal server error"
// @Router /listUsers [get]
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

// @Summary Parse message from request
// @Description Internal function to parse message from HTTP request
// @Tags internal
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
