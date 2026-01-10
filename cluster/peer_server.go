package cluster

import (
	"context"
	"net"
	"time"

	"go.etcd.io/raft/v3/raftpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/cluster/pb"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
)

type peerServer struct {
	pb.UnimplementedPeerServiceServer
	broker   *broker
	address  string
	listener net.Listener
}

// NewServer creates a new server instance
func newPeerServer(broker *broker, address string) *peerServer {
	return &peerServer{broker: broker, address: address}
}
func (s *peerServer) Serve() error {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.listener = lis
	gs := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Second * 20,
			PermitWithoutStream: true,
		}),
	)
	pb.RegisterPeerServiceServer(gs, s)
	return gs.Serve(lis)
}
func (s *peerServer) Shutdown(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
func (s *peerServer) Inspect(ctx context.Context, req *pb.Empty) (*pb.Status, error) {
	return s.broker.inspect(), nil
}
func (s *peerServer) IsOnline(ctx context.Context, req *pb.IsOnlineReq) (*pb.IsOnlineResp, error) {
	ok := s.broker.isActive(req.Targets)
	return &pb.IsOnlineResp{Online: ok}, nil
}

func (s *peerServer) KickConn(ctx context.Context, req *pb.KickConnReq) (*pb.Empty, error) {
	s.broker.kickConn(req.Targets)
	return &pb.Empty{}, nil
}
func (s *peerServer) SendToAll(ctx context.Context, req *pb.MessageReq) (*pb.MessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	total, success := s.broker.sendToAll(ctx, msg)
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *peerServer) SendToTargets(ctx context.Context, req *pb.MessageReq) (*pb.MessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	total, success := s.broker.sendToTargets(ctx, msg, req.Targets)
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *peerServer) SendRaftMessage(ctx context.Context, req *pb.RaftMessage) (*pb.Empty, error) {
	var msg raftpb.Message
	if err := msg.Unmarshal(req.Data); err != nil {
		return nil, err
	}
	if err := s.broker.cluster.Process(ctx, msg); err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}
