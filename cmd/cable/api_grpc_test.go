package main

import (
	"context"
	"testing"

	"sutext.github.io/cable/api/pb"
	"sutext.github.io/cable/cluster"
	clusterpb "sutext.github.io/cable/cluster/pb"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

// TestGRPCMethods tests the gRPC methods directly
func TestGRPCMethods(t *testing.T) {
	// Create a mock broker
	mockBroker := &mockGRPCBroker{
		isOnline: true,
	}

	// Test IsOnline method
	t.Run("TestIsOnline", func(t *testing.T) {
		req := &pb.UserReq{Uid: "testuser"}
		resp, err := mockGRPCServer{broker: mockBroker}.IsOnline(context.Background(), req)
		if err != nil {
			t.Fatalf("IsOnline failed: %v", err)
		}
		if !resp.Ok {
			t.Errorf("Expected user to be online, got offline")
		}
	})

	// Test KickUser method
	t.Run("TestKickUser", func(t *testing.T) {
		req := &pb.UserReq{Uid: "testuser"}
		resp, err := mockGRPCServer{broker: mockBroker}.KickUser(context.Background(), req)
		if err != nil {
			t.Fatalf("KickUser failed: %v", err)
		}
		if resp == nil {
			t.Errorf("Expected non-nil response")
		}
	})

	// Test SendToAll method
	t.Run("TestSendToAll", func(t *testing.T) {
		req := &pb.ToAllReq{
			Qos:     1,
			Kind:    1,
			Message: []byte("test message"),
		}
		resp, err := mockGRPCServer{broker: mockBroker}.SendToAll(context.Background(), req)
		if err != nil {
			t.Fatalf("SendToAll failed: %v", err)
		}
		if resp.Total != 100 || resp.Success != 98 {
			t.Errorf("Expected total=100, success=98, got total=%d, success=%d", resp.Total, resp.Success)
		}
	})

	// Test SendToUser method
	t.Run("TestSendToUser", func(t *testing.T) {
		req := &pb.ToUserReq{
			Uid:     "testuser",
			Qos:     1,
			Kind:    2,
			Message: []byte("test message"),
		}
		resp, err := mockGRPCServer{broker: mockBroker}.SendToUser(context.Background(), req)
		if err != nil {
			t.Fatalf("SendToUser failed: %v", err)
		}
		if resp.Total != 1 || resp.Success != 1 {
			t.Errorf("Expected total=1, success=1, got total=%d, success=%d", resp.Total, resp.Success)
		}
	})
}

// mockGRPCBroker is a mock implementation for gRPC testing
type mockGRPCBroker struct {
	cluster.Broker
	isOnline    bool
	users       []string
	channels    map[string]map[string]string
	testMessage *packet.Message
}

// mockGRPCBooter is a mock implementation of booter for gRPC testing
type mockGRPCBooter struct {
	broker *mockGRPCBroker
}

// mockGRPCServer is a mock implementation of pb.CableServiceServer for testing
type mockGRPCServer struct {
	broker cluster.Broker
}

// Implement the pb.CableServiceServer interface

func (s mockGRPCServer) IsOnline(ctx context.Context, req *pb.UserReq) (*pb.OnlineResp, error) {
	ok := s.broker.IsOnline(ctx, req.Uid)
	return &pb.OnlineResp{
		Ok: ok,
	}, nil
}

func (s mockGRPCServer) KickUser(ctx context.Context, req *pb.UserReq) (*pb.EmptyResp, error) {
	s.broker.KickUser(ctx, req.Uid)
	return &pb.EmptyResp{}, nil
}

func (s mockGRPCServer) SendToAll(ctx context.Context, req *pb.ToAllReq) (*pb.MsgResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.broker.SendToAll(ctx, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}

func (s mockGRPCServer) SendToUser(ctx context.Context, req *pb.ToUserReq) (*pb.MsgResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.broker.SendToUser(ctx, req.Uid, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}

func (s mockGRPCServer) SendToChannel(ctx context.Context, req *pb.ToChannelReq) (*pb.MsgResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.broker.SendToChannel(ctx, req.Channel, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}

func (s mockGRPCServer) JoinChannel(ctx context.Context, req *pb.JoinReq) (*pb.EmptyResp, error) {
	err := s.broker.JoinChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.EmptyResp{}, nil
}

func (s mockGRPCServer) LeaveChannel(ctx context.Context, req *pb.JoinReq) (*pb.EmptyResp, error) {
	err := s.broker.LeaveChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.EmptyResp{}, nil
}

func (s mockGRPCServer) ListChannels(ctx context.Context, req *pb.UserReq) (*pb.ChannelsResp, error) {
	// Mock implementation
	return &pb.ChannelsResp{
		Channels: make(map[string]string),
	}, nil
}

func (s mockGRPCServer) KickNode(ctx context.Context, req *pb.KickNodeReq) (*pb.EmptyResp, error) {
	// Mock implementation
	return &pb.EmptyResp{}, nil
}

func (m *mockGRPCBroker) IsOnline(ctx context.Context, userID string) bool {
	return m.isOnline
}

func (m *mockGRPCBroker) KickUser(ctx context.Context, userID string) {
	// Mock implementation
}

func (m *mockGRPCBroker) SendToAll(ctx context.Context, msg *packet.Message) (int32, int32, error) {
	m.testMessage = msg
	return 100, 98, nil
}

func (m *mockGRPCBroker) SendToUser(ctx context.Context, userID string, msg *packet.Message) (int32, int32, error) {
	m.testMessage = msg
	return 1, 1, nil
}

func (m *mockGRPCBroker) SendToChannel(ctx context.Context, channel string, msg *packet.Message) (int32, int32, error) {
	m.testMessage = msg
	return 50, 48, nil
}

func (m *mockGRPCBroker) JoinChannel(ctx context.Context, userID string, channels map[string]string) error {
	if _, ok := m.channels[userID]; !ok {
		m.channels[userID] = make(map[string]string)
	}
	for ch, role := range channels {
		m.channels[userID][ch] = role
	}
	return nil
}

func (m *mockGRPCBroker) LeaveChannel(ctx context.Context, userID string, channels map[string]string) error {
	if _, ok := m.channels[userID]; ok {
		for ch := range channels {
			delete(m.channels[userID], ch)
		}
	}
	return nil
}

func (m *mockGRPCBroker) ListUsers(ctx context.Context, brokerID uint64) (map[string]map[string]string, error) {
	result := make(map[string]map[string]string)
	for _, user := range m.users {
		result[user] = make(map[string]string)
	}
	return result, nil
}

func (m *mockGRPCBroker) Cluster() cluster.Cluster {
	return &mockGRPCCluster{ready: true}
}

func (m *mockGRPCBroker) Inspects(ctx context.Context) ([]*clusterpb.Status, error) {
	return []*clusterpb.Status{}, nil
}

func (m *mockGRPCBroker) Start() {
	// Mock implementation
}

func (m *mockGRPCBroker) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockGRPCBroker) HandleRequest(method string, handler server.RequestHandler) {
	// Mock implementation
}

// mockGRPCCluster is a mock implementation of cluster.Cluster for gRPC testing
type mockGRPCCluster struct {
	cluster.Cluster
	ready bool
}

func (m *mockGRPCCluster) IsReady() bool {
	return m.ready
}

func (m *mockGRPCCluster) KickBroker(ctx context.Context, id uint64) error {
	return nil
}

func (m *mockGRPCCluster) AddBroker(ctx context.Context, id uint64, addr string) error {
	return nil
}

func (m *mockGRPCBooter) ListChannels(ctx context.Context, userID string) (map[string]string, error) {
	if channels, ok := m.broker.channels[userID]; ok {
		return channels, nil
	}
	return make(map[string]string), nil
}
