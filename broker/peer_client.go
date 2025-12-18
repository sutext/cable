package broker

import (
	"context"
	"fmt"
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type peerClient struct {
	id     string
	ip     string
	ipmu   sync.Mutex
	broker *broker
	logger *xlog.Logger
	cli    protos.PeerServiceClient
	conn   *grpc.ClientConn
}

func newPeerClient(id, ip string, broker *broker) *peerClient {
	p := &peerClient{
		id:     id,
		ip:     ip,
		broker: broker,
	}
	return p
}
func (p *peerClient) connect() {
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
func (p *peerClient) IsReady() bool {
	return true
}
func (p *peerClient) UpdateIP(ip string) {
	p.ipmu.Lock()
	defer p.ipmu.Unlock()
	if p.ip == ip {
		p.connect()
		return
	}
	p.broker.logger.Warn("peer ip updated", xlog.Str("ip", ip))
	p.ip = ip
	p.connect()
}
func (p *peerClient) sendMessage(ctx context.Context, m *packet.Message, target string, flag uint8) (total, success int32, err error) {
	data, err := coder.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	req := &protos.SendMessageReq{
		Flag:    int32(flag),
		Target:  target,
		Message: data,
	}
	resp, err := p.cli.SendMessage(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}

func (p *peerClient) isOnline(ctx context.Context, uid string) (bool, error) {
	resp, err := p.cli.IsOnline(ctx, &protos.IsOnlineReq{Uid: uid})
	if err != nil {
		return false, err
	}
	return resp.Online, nil
}
func (p *peerClient) kickConn(ctx context.Context, cid string) error {
	_, err := p.cli.KickConn(ctx, &protos.KickConnReq{Cid: cid})
	return err
}
func (p *peerClient) kickUser(ctx context.Context, uid string) error {
	_, err := p.cli.KickUser(ctx, &protos.KickUserReq{Uid: uid})
	return err
}
func (p *peerClient) inspect(ctx context.Context) (*protos.Inspects, error) {
	return p.cli.Inspect(ctx, &protos.Empty{})
}

func (p *peerClient) joinChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
	resp, err := p.cli.JoinChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func (p *peerClient) leaveChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
	resp, err := p.cli.LeaveChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}
