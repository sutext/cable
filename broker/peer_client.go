package broker

import (
	"context"
	"fmt"
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type peer_client struct {
	id     string
	ip     string
	ipmu   sync.Mutex
	broker *broker
	logger *xlog.Logger
	cli    protos.PeerServiceClient
	conn   *grpc.ClientConn
}

func newPeerClient(id, ip string, broker *broker) *peer_client {
	p := &peer_client{
		id:     id,
		ip:     ip,
		broker: broker,
		logger: xlog.With("GROUP", "PEER", "peerid", id),
	}
	return p
}
func (p *peer_client) connect() {
	// Set up a connection to the server.
	conn, err := grpc.NewClient(fmt.Sprintf("%s%s", p.ip, p.broker.peerPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = conn
	p.cli = protos.NewPeerServiceClient(conn)
}
func (p *peer_client) IsReady() bool {
	return true
}
func (p *peer_client) UpdateIP(ip string) {
	p.ipmu.Lock()
	defer p.ipmu.Unlock()
	if p.ip == ip {
		p.connect()
		return
	}
	p.logger.Warn("peer ip updated", xlog.Str("ip", ip))
	p.ip = ip
	p.connect()
}
func (p *peer_client) sendMessage(ctx context.Context, m *packet.Message, target string, flag uint8) (total, success uint64, err error) {
	return p.sendMessage(ctx, m, target, flag)
}

func (p *peer_client) isOnline(ctx context.Context, uid string) (bool, error) {
	resp, err := p.cli.IsOnline(ctx, &protos.IsOnlineReq{Uid: uid})
	if err != nil {
		return false, err
	}
	return resp.Online, nil
}
func (p *peer_client) kickConn(ctx context.Context, cid string) error {
	_, err := p.cli.KickConn(ctx, &protos.KickConnReq{Cid: cid})
	return err
}
func (p *peer_client) kickUser(ctx context.Context, uid string) error {
	_, err := p.cli.KickUser(ctx, &protos.KickUserReq{Uid: uid})
	return err
}
func (p *peer_client) inspect(ctx context.Context) (*protos.InspectResp, error) {
	return p.cli.Inspect(ctx, &protos.InspectReq{})
}

func (p *peer_client) joinChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
	resp, err := p.cli.JoinChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func (p *peer_client) leaveChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
	resp, err := p.cli.LeaveChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}
