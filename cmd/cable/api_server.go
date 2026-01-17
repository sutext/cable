package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/api/pb"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

type apiServer struct {
	pb.UnimplementedCableServiceServer
	booter   *booter
	broker   cluster.Broker
	address  string
	listener net.Listener
}

func newAPIServer(booter *booter) *apiServer {
	return &apiServer{
		booter:  booter,
		broker:  booter.broker,
		address: fmt.Sprintf(":%d", booter.config.GrpcPort)}
}

func (s *apiServer) Serve() error {
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
	pb.RegisterCableServiceServer(gs, s)
	return gs.Serve(lis)
}

func (s *apiServer) IsOnline(ctx context.Context, req *pb.UserReq) (*pb.UserResp, error) {
	ok := s.broker.IsOnline(ctx, req.Uid)
	return &pb.UserResp{
		Ok: ok,
	}, nil
}
func (s *apiServer) KickUser(ctx context.Context, req *pb.UserReq) (*pb.UserResp, error) {
	s.broker.KickUser(ctx, req.Uid)
	return &pb.UserResp{
		Ok: true,
	}, nil
}
func (s *apiServer) SendToAll(ctx context.Context, req *pb.ToAllReq) (*pb.MessageResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.broker.SendToAll(ctx, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *apiServer) SendToUser(ctx context.Context, req *pb.ToUserReq) (*pb.MessageResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.broker.SendToUser(ctx, req.Uid, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *apiServer) SendToChannel(ctx context.Context, req *pb.ToChannelReq) (*pb.MessageResp, error) {
	msg := &packet.Message{
		Qos:     packet.MessageQos(req.Qos),
		Kind:    packet.MessageKind(req.Kind),
		Payload: req.Message,
	}
	total, success, err := s.broker.SendToChannel(ctx, req.Channel, msg)
	if err != nil {
		return nil, err
	}
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}
func (s *apiServer) JoinChannel(ctx context.Context, req *pb.JoinReq) (*pb.Empty, error) {
	if s.booter.redis != nil {
		s.booter.redis.HSet(ctx, s.booter.userKey(req.Uid), "channels", req.Channels)
	}
	err := s.broker.JoinChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}
func (s *apiServer) LeaveChannel(ctx context.Context, req *pb.JoinReq) (*pb.Empty, error) {
	if s.booter.redis != nil {
		s.booter.redis.HDel(ctx, s.booter.userKey(req.Uid), "channels")
	}
	err := s.broker.LeaveChannel(ctx, req.Uid, req.Channels)
	if err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}

func (s *apiServer) GetChannels(ctx context.Context, req *pb.UserReq) (*pb.Channels, error) {
	channels, err := s.booter.redis.HGetAll(ctx, s.booter.userKey(req.Uid)).Result()
	if err != nil {
		return nil, err
	}
	return &pb.Channels{
		Channels: channels,
	}, nil
}
