package broker

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
)

type peerServer struct {
	protos.UnimplementedPeerServiceServer
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
	protos.RegisterPeerServiceServer(gs, s)
	return gs.Serve(lis)
}
func (s *peerServer) Shutdown(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
func (s *peerServer) Inspect(ctx context.Context, req *protos.Empty) (*protos.Inspects, error) {
	return s.broker.inspect(), nil
}
func (s *peerServer) IsOnline(ctx context.Context, req *protos.IsOnlineReq) (*protos.IsOnlineResp, error) {
	ok := s.broker.isActive(req.Targets)
	return &protos.IsOnlineResp{Online: ok}, nil
}

func (s *peerServer) KickConn(ctx context.Context, req *protos.KickConnReq) (*protos.Empty, error) {
	s.broker.kickConn(req.Targets)
	return &protos.Empty{}, nil
}

//	func (s *peerServer) JoinChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
//		count := s.broker.joinChannel(req.Uid, req.Channels)
//		return &protos.ChannelResp{Count: count}, nil
//	}
//
//	func (s *peerServer) LeaveChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
//		count := s.broker.leaveChannel(req.Uid, req.Channels)
//		return &protos.ChannelResp{Count: count}, nil
//	}
func (s *peerServer) SendToAll(ctx context.Context, req *protos.MessageReq) (*protos.MessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	total, success := s.broker.sendToAll(ctx, msg)
	return &protos.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *peerServer) SendToTargets(ctx context.Context, req *protos.MessageReq) (*protos.MessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	total, success := s.broker.sendToTargets(ctx, msg, req.Targets)
	return &protos.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}

// HandleRaftMessage 处理来自其他节点的Raft消息
func (s *peerServer) HandleRaftMessage(ctx context.Context, req *protos.RaftMessage) (*protos.Empty, error) {
	// 暂时简化实现，仅记录日志
	s.broker.logger.Debug("Received Raft message")
	// 注意：此处需要实现正确的Raft消息解码和处理逻辑
	// 但目前由于API不一致问题，暂时注释掉
	/*
		// 解码Raft消息
		var msg raftpb.Message
		// 使用正确的方式解码Raft消息
		s.broker.node.Step(ctx, msg)
	*/
	return &protos.Empty{}, nil
}
