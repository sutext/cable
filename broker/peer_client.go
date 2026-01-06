package broker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/raft/v3/raftpb"
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
	id     uint64
	mu     sync.Mutex
	rpc    protos.PeerServiceClient
	addr   string
	port   string
	conn   *grpc.ClientConn
	ready  atomic.Bool
	logger *xlog.Logger
}

func newPeerClient(id uint64, addr string, logger *xlog.Logger) *peerClient {
	p := &peerClient{
		id:     id,
		addr:   addr,
		logger: logger,
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
		p.logger.Error("connect to peer failed", xlog.Err(err), xlog.Peer(p.id))
		time.AfterFunc(time.Second*2, func() {
			p.connect()
		})
	} else {
		p.logger.Info("connect to peer success", xlog.Peer(p.id))
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
	conn, err := grpc.NewClient(p.addr,
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
func (p *peerClient) Close() {
	if p.ready.CompareAndSwap(true, false) {
		if p.conn != nil {
			p.conn.Close()
			p.conn = nil
		}
		p.logger.Info("peer client closed", xlog.Peer(p.id))
	}

}
func (p *peerClient) isReady() bool {
	return p.ready.Load()
}
func (p *peerClient) updateAddr(addr string) {
	if p.addr == addr {
		return
	}
	p.logger.Warn("peer ip updated", xlog.Str("addr", addr))
	p.addr = addr
	p.ready.Store(false)
	p.connect()
}
func (p *peerClient) sendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error) {
	if !p.isReady() {
		return 0, 0, xerr.PeerNotReady
	}
	data, err := coder.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	req := &protos.MessageReq{
		Message: data,
	}
	resp, err := p.rpc.SendToAll(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}
func (p *peerClient) sendToTargets(ctx context.Context, m *packet.Message, tragets []*protos.Target) (total, success int32, err error) {
	if !p.isReady() {
		return 0, 0, xerr.PeerNotReady
	}
	data, err := coder.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	req := &protos.MessageReq{
		Message: data,
		Targets: tragets,
	}
	resp, err := p.rpc.SendToTargets(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}
func (p *peerClient) isOnline(ctx context.Context, targets []*protos.Target) (bool, error) {
	if !p.isReady() {
		return false, xerr.PeerNotReady
	}
	resp, err := p.rpc.IsOnline(ctx, &protos.IsOnlineReq{Targets: targets})
	if err != nil {
		return false, err
	}
	return resp.Online, nil
}
func (p *peerClient) kickConn(ctx context.Context, targets []*protos.Target) error {
	if !p.isReady() {
		return xerr.PeerNotReady
	}
	_, err := p.rpc.KickConn(ctx, &protos.KickConnReq{Targets: targets})
	return err
}
func (p *peerClient) inspect(ctx context.Context) (*protos.Inspects, error) {
	if !p.isReady() {
		return nil, xerr.PeerNotReady
	}
	return p.rpc.Inspect(ctx, &protos.Empty{})
}

// sendRaftMessage 发送Raft消息到远程节点
func (p *peerClient) sendRaftMessage(ctx context.Context, msg raftpb.Message) error {
	if !p.isReady() {
		return xerr.PeerNotReady
	}
	data, err := msg.Marshal()
	if err != nil {
		return err
	}
	req := &protos.RaftMessage{
		Data: data,
	}
	_, err = p.rpc.SendRaftMessage(ctx, req)
	return err
}

// func (p *peerClient) joinChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
// 	if !p.isReady() {
// 		return 0, xerr.PeerNotReady
// 	}
// 	resp, err := p.rpc.JoinChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
// 	if err != nil {
// 		return 0, err
// 	}
// 	return resp.Count, nil
// }

// func (p *peerClient) leaveChannel(ctx context.Context, uid string, channels []string) (count int32, err error) {
// 	if !p.isReady() {
// 		return 0, xerr.PeerNotReady
// 	}
// 	resp, err := p.rpc.LeaveChannel(ctx, &protos.ChannelReq{Uid: uid, Channels: channels})
// 	if err != nil {
// 		return 0, err
// 	}
// 	return resp.Count, nil
// }
