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

type PeerServer struct {
	broker   *broker
	address  string
	listener net.Listener
	protos.UnimplementedPeerServiceServer
}

// NewServer creates a new server instance
func NewPeerServer(broker *broker, address string) *PeerServer {
	return &PeerServer{broker: broker, address: address}
}
func (s *PeerServer) Serve() error {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.listener = lis
	gs := grpc.NewServer()
	protos.RegisterPeerServiceServer(gs, s)
	return gs.Serve(lis)
}
func (s *PeerServer) Shutdown(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
func (s *PeerServer) Inspect(ctx context.Context, req *protos.InspectReq) (*protos.InspectResp, error) {
	return s.broker.inspect(), nil
}
func (s *PeerServer) IsOnline(ctx context.Context, req *protos.IsOnlineReq) (*protos.IsOnlineResp, error) {
	ok := s.broker.isOnline(req.Uid)
	return &protos.IsOnlineResp{Online: ok}, nil
}
func (s *PeerServer) JoinChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
	count := s.broker.joinChannel(req.Uid, req.Channels)
	return &protos.ChannelResp{Count: count}, nil
}
func (s *PeerServer) KickConn(ctx context.Context, req *protos.KickConnReq) (*protos.EmptyResp, error) {
	s.broker.kickConn(req.Cid)
	return &protos.EmptyResp{}, nil
}
func (s *PeerServer) KickUser(ctx context.Context, req *protos.KickUserReq) (*protos.EmptyResp, error) {
	s.broker.kickUser(req.Uid)
	return &protos.EmptyResp{}, nil
}
func (s *PeerServer) LeaveChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
	count := s.broker.leaveChannel(req.Uid, req.Channels)
	return &protos.ChannelResp{Count: count}, nil
}
func (s *PeerServer) SendMessage(ctx context.Context, req *protos.SendMessageReq) (*protos.SendMessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	var total, success int32
	switch req.Flag {
	case 0:
		total, success = s.broker.sendToAll(msg)
	case 1:
		total, success = s.broker.sendToUser(req.Target, msg)
	case 2:
		total, success = s.broker.sendToChannel(req.Target, msg)
	default:
		return nil, xerr.InvalidPeerMessageFlag
	}
	return &protos.SendMessageResp{
		Total:   total,
		Success: success,
	}, nil

}
