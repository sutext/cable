// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
package cluster

import (
	"context"
	"fmt"
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

// peerServer implements the gRPC PeerServiceServer interface for peer-to-peer communication.
type peerServer struct {
	pb.UnimplementedPeerServiceServer              // Embeds unimplemented gRPC server for forward compatibility
	broker                            *broker      // Reference to the parent broker
	address                           string       // Address to listen on
	server                            *grpc.Server // Network listener
}

// newPeerServer creates a new peer server instance.
//
// Parameters:
// - broker: Parent broker instance
// - address: Address to listen on (e.g., ":4567")
//
// Returns:
// - *peerServer: A new peer server instance
func newPeerServer(broker *broker, opts *options) *peerServer {
	return &peerServer{
		broker:  broker,
		address: fmt.Sprintf(":%d", opts.peerPort),
		server: grpc.NewServer(
			grpc.StatsHandler(opts.grpcStatsHandler.Server),
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             time.Second * 20,
				PermitWithoutStream: true,
			}),
		),
	}
}

// Serve starts the gRPC server and begins listening for incoming requests.
//
// Returns:
// - error: Error if starting the server fails, nil otherwise
func (s *peerServer) Serve() error {
	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	pb.RegisterPeerServiceServer(s.server, s)
	return s.server.Serve(lis)
}

// Shutdown shuts down the gRPC server and closes the listener.
//
// Parameters:
// - ctx: Context for the shutdown operation
//
// Returns:
// - error: Error if shutting down the server fails, nil otherwise
func (s *peerServer) Shutdown(ctx context.Context) error {
	s.server.GracefulStop()
	return nil
}

// Inspect returns the status of the broker.
//
// Parameters:
// - ctx: Context for the request
// - req: Empty request
//
// Returns:
// - *pb.Status: Broker status
// - error: Error if inspecting fails, nil otherwise
func (s *peerServer) Inspect(ctx context.Context, req *pb.Empty) (*pb.Status, error) {
	return s.broker.inspect(), nil
}

// IsOnline checks if any of the specified targets are online.
//
// Parameters:
// - ctx: Context for the request
// - req: Request containing target clients to check
//
// Returns:
// - *pb.IsOnlineResp: Response indicating if any target is online
// - error: Error if checking fails, nil otherwise
func (s *peerServer) IsOnline(ctx context.Context, req *pb.IsOnlineReq) (*pb.IsOnlineResp, error) {
	ok := s.broker.isActive(req.Targets)
	return &pb.IsOnlineResp{Online: ok}, nil
}

// KickConn kicks the specified target clients.
//
// Parameters:
// - ctx: Context for the request
// - req: Request containing target clients to kick
//
// Returns:
// - *pb.Empty: Empty response
// - error: Error if kicking fails, nil otherwise
func (s *peerServer) KickConn(ctx context.Context, req *pb.KickConnReq) (*pb.Empty, error) {
	s.broker.kickConn(req.Targets)
	return &pb.Empty{}, nil
}

// SendToAll sends a message to all clients on this broker.
//
// Parameters:
// - ctx: Context for the request
// - req: Request containing the message to send
//
// Returns:
// - *pb.MessageResp: Response containing send statistics
// - error: Error if sending fails, nil otherwise
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

// SendToTargets sends a message to specific target clients on this broker.
//
// Parameters:
// - ctx: Context for the request
// - req: Request containing the message and target clients
//
// Returns:
// - *pb.MessageResp: Response containing send statistics
// - error: Error if sending fails, nil otherwise
func (s *peerServer) SendToUser(ctx context.Context, req *pb.MessageReq) (*pb.MessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	total, success := s.broker.sendToClients(ctx, msg, req.Targets)
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}

// SendToChannel sends a message to specific channel clients on this broker.
//
// Parameters:
// - ctx: Context for the request
// - req: Request containing the message and target clients
//
// Returns:
// - *pb.MessageResp: Response containing send statistics
// - error: Error if sending fails, nil otherwise
func (s *peerServer) SendToChannel(ctx context.Context, req *pb.MessageReq) (*pb.MessageResp, error) {
	msg := &packet.Message{}
	if err := coder.Unmarshal(req.Message, msg); err != nil {
		return nil, xerr.InvalidPeerMessage
	}
	total, success := s.broker.sendToClients(ctx, msg, req.Targets)
	return &pb.MessageResp{
		Total:   total,
		Success: success,
	}, nil
}

// SendRaftMessage processes a Raft message from a peer.
//
// Parameters:
// - ctx: Context for the request
// - req: Request containing the Raft message
//
// Returns:
// - *pb.Empty: Empty response
// - error: Error if processing fails, nil otherwise
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
