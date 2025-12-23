package broker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type peerClient struct {
	id     string
	ip     string
	mu     sync.Mutex
	rpc    protos.PeerServiceClient
	conn   *grpc.ClientConn
	ready  atomic.Bool
	broker *broker
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ready.Load() {
		return
	}
	if err := p.reconnnect(); err != nil {
		p.broker.logger.Error("connect to peer failed", xlog.Err(err), xlog.Peer(p.id))
		time.AfterFunc(time.Second*2, func() {
			p.connect()
		})
	} else {
		p.broker.logger.Info("connect to peer success", xlog.Peer(p.id))
		p.ready.Store(true)
	}
}
func (p *peerClient) reconnnect() error {
	config := `{
  		"loadBalancingPolicy": "round_robin",
 		"methodConfig": [{
    		"name": [{"service": "protos.PeerService"}],
    		"waitForReady": true,
    		"timeout": "3s",
    		"retryPolicy": {
      			"maxAttempts": 3,
      			"initialBackoff": "0.1s",
      			"maxBackoff": "1s",
      			"backoffMultiplier": 2,
      			"retryableStatusCodes": ["UNAVAILABLE"]
    		}
  		}]
	}`
	conn, err := grpc.NewClient(
		fmt.Sprintf("%s%s", p.ip, p.broker.peerPort),
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				MaxDelay:   20 * time.Second,
				Multiplier: 1.2,
				Jitter:     0.2,
			},
		}),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithNoProxy(),
		grpc.WithDefaultServiceConfig(config),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = conn
	p.rpc = protos.NewPeerServiceClient(conn)
	return nil
}
func (p *peerClient) isReady() bool {
	return p.ready.Load()
}
func (p *peerClient) updateIP(ip string) {
	if p.ip == ip {
		return
	}
	p.broker.logger.Warn("peer ip updated", xlog.Str("ip", ip))
	p.ip = ip
	p.ready.Store(false)
	p.connect()
}
func (p *peerClient) sendMessage(ctx context.Context, m *packet.Message, target string, flag uint8) (total, success int32, err error) {
	if !p.isReady() {
		return 0, 0, xerr.PeerNotReady
	}
	data, err := coder.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	req := &protos.SendMessageReq{
		Flag:    int32(flag),
		Target:  target,
		Message: data,
	}
	resp, err := p.rpc.SendMessage(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}

func (p *peerClient) isOnline(ctx context.Context, uid string) (bool, error) {
	if !p.isReady() {
		return false, xerr.PeerNotReady
	}
	resp, err := p.rpc.IsOnline(ctx, &protos.IsOnlineReq{Uid: uid})
	if err != nil {
		return false, err
	}
	return resp.Online, nil
}
func (p *peerClient) kickConn(ctx context.Context, cid string) error {
	if !p.isReady() {
		return xerr.PeerNotReady
	}
	_, err := p.rpc.KickConn(ctx, &protos.KickConnReq{Cid: cid})
	return err
}
func (p *peerClient) kickUser(ctx context.Context, uid string) error {
	if !p.isReady() {
		return xerr.PeerNotReady
	}
	_, err := p.rpc.KickUser(ctx, &protos.KickUserReq{Uid: uid})
	return err
}
func (p *peerClient) inspect(ctx context.Context) (*protos.Inspects, error) {
	if !p.isReady() {
		return nil, xerr.PeerNotReady
	}
	return p.rpc.Inspect(ctx, &protos.Empty{})
}

func (p *peerClient) joinChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
	if !p.isReady() {
		return 0, xerr.PeerNotReady
	}
	resp, err := p.rpc.JoinChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func (p *peerClient) leaveChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
	if !p.isReady() {
		return 0, xerr.PeerNotReady
	}
	resp, err := p.rpc.LeaveChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}
