package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/api/pb"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

type grpcServer struct {
	pb.UnimplementedCableServiceServer
	booter   *booter
	broker   cluster.Broker
	address  string
	listener net.Listener
}

func newGRPC(booter *booter) *grpcServer {
	return &grpcServer{
		booter:  booter,
		broker:  booter.broker,
		address: fmt.Sprintf(":%d", booter.config.GrpcPort)}
}

func (s *grpcServer) Serve() error {
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
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
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
	total, success, err := s.broker.SendToAll(ctx, msg)
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
	total, success, err := s.broker.SendToUser(ctx, req.Uid, msg)
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
	total, success, err := s.broker.SendToChannel(ctx, req.Channel, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MsgResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *grpcServer) JoinChannel(ctx context.Context, req *pb.JoinReq) (*pb.EmptyResp, error) {
	if s.booter.redis != nil {
		s.booter.redis.HSet(ctx, s.booter.userKey(req.Uid), "channels", req.Channels)
	}
	err := s.broker.JoinChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.EmptyResp{}, nil
}
func (s *grpcServer) LeaveChannel(ctx context.Context, req *pb.JoinReq) (*pb.EmptyResp, error) {
	if s.booter.redis != nil {
		s.booter.redis.HDel(ctx, s.booter.userKey(req.Uid), "channels")
	}
	err := s.broker.LeaveChannel(ctx, req.Uid, req.Channels)
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
