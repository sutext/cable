// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
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

// inspect returns the status of the local broker.
// It includes information about user count, channel count, and Raft status.
//
// Returns:
// - *pb.Status: Status information of the local broker
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
	status, _ := b.cluster.Status()
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

// Inspects returns the status of all brokers in the cluster.
// It collects status information from the local broker and all peer brokers.
//
// Parameters:
// - ctx: Context for the operation
//
// Returns:
// - []*pb.Status: List of status information for all brokers in the cluster
// - error: Error if inspecting any broker fails, nil otherwise
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

// handleInspect handles HTTP requests to inspect the cluster status.
// It returns status information for all brokers in the cluster as JSON.
//
// Parameters:
// - w: HTTP response writer
// - r: HTTP request
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

// handleKickUser handles HTTP requests to kick a user from all connections in the cluster.
// It requires a 'uid' query parameter specifying the user ID to kick.
//
// Parameters:
// - w: HTTP response writer
// - r: HTTP request
func (b *broker) handleKickUser(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	b.KickUser(r.Context(), uid)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleSendMessage handles HTTP requests to send a message to users, channels, or all clients.
// It supports sending to a specific user (via 'uid' query parameter), a specific channel (via 'channel' query parameter),
// or all clients (if no uid or channel is specified). The 'msg' query parameter is required.
//
// Parameters:
// - w: HTTP response writer
// - r: HTTP request
func (b *broker) handleSendMessage(w http.ResponseWriter, r *http.Request) {
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

// handleJoin handles HTTP requests to add a user to one or more channels.
// It requires 'uid' and 'channels' query parameters, where 'channels' is a comma-separated list of channel names.
//
// Parameters:
// - w: HTTP response writer
// - r: HTTP request
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
	strs := strings.Split(chs, ",")
	channels := make(map[string]string)
	for _, ch := range strs {
		channels[ch] = ""
	}
	err := b.JoinChannel(r.Context(), uid, channels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleHealth handles HTTP requests to check the health of the broker.
// It returns HTTP 200 OK if the broker is ready, otherwise returns HTTP 500 Internal Server Error.
//
// Parameters:
// - w: HTTP response writer
// - r: HTTP request
func (b *broker) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !b.cluster.IsReady() {
		http.Error(w, "broker is not ready", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleKickNode handles HTTP requests to remove a broker from the cluster.
// It requires a 'nodeId' query parameter specifying the broker ID to remove.
//
// Parameters:
// - w: HTTP response writer
// - r: HTTP request
func (b *broker) handleKickNode(w http.ResponseWriter, r *http.Request) {
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
	err = b.cluster.KickBroker(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
