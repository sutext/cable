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
func (s *peerServer) UserOpened(ctx context.Context, req *protos.UserOpenedReq) (*protos.Empty, error) {
	s.broker.userOpened(req)
	return &protos.Empty{}, nil
}
func (s *peerServer) UserClosed(ctx context.Context, req *protos.UserClosedReq) (*protos.Empty, error) {
	s.broker.userClosed(req)
	return &protos.Empty{}, nil
}
