package broker

import (
	"context"
	"net"

	"google.golang.org/grpc"
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
	gs := grpc.NewServer()
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
	ok := s.broker.isOnline(req.Uid)
	return &protos.IsOnlineResp{Online: ok}, nil
}
func (s *peerServer) JoinChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
	count := s.broker.joinChannel(req.Uid, req.Channels)
	return &protos.ChannelResp{Count: count}, nil
}
func (s *peerServer) KickConn(ctx context.Context, req *protos.KickConnReq) (*protos.Empty, error) {
	s.broker.kickConn(req.Cid)
	return &protos.Empty{}, nil
}
func (s *peerServer) KickUser(ctx context.Context, req *protos.KickUserReq) (*protos.Empty, error) {
	s.broker.kickUser(req.Uid)
	return &protos.Empty{}, nil
}
func (s *peerServer) LeaveChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
	count := s.broker.leaveChannel(req.Uid, req.Channels)
	return &protos.ChannelResp{Count: count}, nil
}
func (s *peerServer) SendMessage(ctx context.Context, req *protos.SendMessageReq) (*protos.SendMessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	var total, success int32
	switch req.Flag {
	case 0:
		total, success = s.broker.sendToAll(ctx, msg)
	case 1:
		total, success = s.broker.sendToUser(ctx, req.Target, msg)
	case 2:
		total, success = s.broker.sendToChannel(ctx, req.Target, msg)
	default:
		return nil, xerr.InvalidPeerMessageFlag
	}
	return &protos.SendMessageResp{
		Total:   total,
		Success: success,
	}, nil

}
