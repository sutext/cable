package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/stats"
	"sutext.github.io/cable/api/pb"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

type grpcServer struct {
	pb.UnimplementedCableServiceServer
	booter   *booter
	broker   cluster.Broker
	listener net.Listener
}

func newGRPC(booter *booter) *grpcServer {
	return &grpcServer{
		booter: booter,
		broker: booter.broker,
	}
}

func (s *grpcServer) Serve() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.booter.config.GrpcPort))
	if err != nil {
		return err
	}
	s.listener = lis
	var h stats.Handler
	if s.booter.config.Trace.Enabled || s.booter.config.Metrics.Enabled {
		h = otelgrpc.NewServerHandler(
			otelgrpc.WithPublicEndpoint(),
		)
	}
	gs := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Second * 20,
			PermitWithoutStream: true,
		}),
		grpc.StatsHandler(h),
	)
	pb.RegisterCableServiceServer(gs, s)
	return gs.Serve(lis)
}

func (s *grpcServer) IsOnline(ctx context.Context, req *pb.UserReq) (*pb.OnlineResp, error) {
	ok := s.broker.IsOnline(ctx, req.Uid)
	return &pb.OnlineResp{
		Ok: ok,
	}, nil
}
func (s *grpcServer) KickUser(ctx context.Context, req *pb.UserReq) (*pb.EmptyResp, error) {
	s.broker.KickUser(ctx, req.Uid)
	return &pb.EmptyResp{}, nil
}
func (s *grpcServer) SendToAll(ctx context.Context, req *pb.ToAllReq) (*pb.MsgResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.booter.SendToAll(ctx, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *grpcServer) SendToUser(ctx context.Context, req *pb.ToUserReq) (*pb.MsgResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.booter.SendToUser(ctx, req.Uid, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *grpcServer) SendToChannel(ctx context.Context, req *pb.ToChannelReq) (*pb.MsgResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.booter.SendToChannel(ctx, req.Channel, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *grpcServer) JoinChannel(ctx context.Context, req *pb.JoinReq) (*pb.EmptyResp, error) {
	err := s.booter.JoinChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.EmptyResp{}, nil
}
func (s *grpcServer) LeaveChannel(ctx context.Context, req *pb.JoinReq) (*pb.EmptyResp, error) {
	err := s.booter.LeaveChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.EmptyResp{}, nil
}

func (s *grpcServer) ListChannels(ctx context.Context, req *pb.UserReq) (*pb.ChannelsResp, error) {
	channels, err := s.booter.ListChannels(ctx, req.Uid)
	if err != nil {
		return nil, err
	}
	return &pb.ChannelsResp{
		Channels: channels,
	}, nil
}
