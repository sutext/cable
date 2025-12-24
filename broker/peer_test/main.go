package main

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/broker/protos"
)

type emptyGrpcServer struct {
	protos.UnimplementedPeerServiceServer
	address  string
	listener net.Listener
}

func newTestServer(address string) *emptyGrpcServer {
	return &emptyGrpcServer{address: address}
}
func (s *emptyGrpcServer) Serve() error {
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
func (s *emptyGrpcServer) Shutdown(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
func (s *emptyGrpcServer) Inspect(ctx context.Context, req *protos.Empty) (*protos.Inspects, error) {
	return &protos.Inspects{}, nil
}
func (s *emptyGrpcServer) IsOnline(ctx context.Context, req *protos.IsOnlineReq) (*protos.IsOnlineResp, error) {
	return &protos.IsOnlineResp{Online: false}, nil
}
func (s *emptyGrpcServer) JoinChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
	return &protos.ChannelResp{Count: 0}, nil
}
func (s *emptyGrpcServer) KickConn(ctx context.Context, req *protos.KickConnReq) (*protos.Empty, error) {
	return &protos.Empty{}, nil
}
func (s *emptyGrpcServer) KickUser(ctx context.Context, req *protos.KickUserReq) (*protos.Empty, error) {
	return &protos.Empty{}, nil
}
func (s *emptyGrpcServer) LeaveChannel(ctx context.Context, req *protos.ChannelReq) (*protos.ChannelResp, error) {
	return &protos.ChannelResp{Count: 0}, nil
}
func (s *emptyGrpcServer) SendMessage(ctx context.Context, req *protos.SendMessageReq) (*protos.SendMessageResp, error) {
	return &protos.SendMessageResp{
		Total:   0,
		Success: 0,
	}, nil

}
func main() {
	s := newTestServer("localhost:4567")
	s.Serve()
}
