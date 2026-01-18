package api

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"sutext.github.io/cable/api/pb"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

var (
	ErrClientNotReady = errors.New("client not ready")
)

type Client interface {
	Close()
	Connect()
	IsReady() bool
	IsOnline(ctx context.Context, uid string) (ok bool, err error)
	KickUser(ctx context.Context, uid string) (err error)
	SendToAll(ctx context.Context, qos packet.MessageQos, kind packet.MessageKind, message []byte) (total, success int32, err error)
	SendToUser(ctx context.Context, uid string, qos packet.MessageQos, kind packet.MessageKind, message []byte) (total, success int32, err error)
	SendToChannel(ctx context.Context, channel string, qos packet.MessageQos, kind packet.MessageKind, message []byte) (total, success int32, err error)
	JoinChannel(ctx context.Context, uid string, channels map[string]string) (err error)
	LeaveChannel(ctx context.Context, uid string, channels map[string]string) (err error)
	GetChannels(ctx context.Context, uid string) (channels map[string]string, err error)
}
type client struct {
	mu     sync.Mutex
	rpc    pb.CableServiceClient
	conn   *grpc.ClientConn
	addr   string
	ready  atomic.Bool
	logger *xlog.Logger
}

func NewClient(addr string) Client {
	return &client{
		addr:   addr,
		logger: xlog.With("API"),
	}
}

func (c *client) Connect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready.Load() {
		return
	}
	if err := c.reconnnect(); err != nil {
		time.AfterFunc(time.Second*2, func() {
			c.Connect()
		})
	} else {
		c.ready.Store(true)
	}
}

func (c *client) reconnnect() error {
	config := `{
  "loadBalancingPolicy": "round_robin",
 "methodConfig": [{
    "name": [{"service": "pb.CableService"}],
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
	conn, err := grpc.NewClient(c.addr,
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
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = conn
	c.rpc = pb.NewCableServiceClient(conn)
	return nil
}
func (c *client) Close() {
	if c.ready.CompareAndSwap(true, false) {
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
	}
}
func (c *client) IsReady() bool {
	return c.ready.Load()
}
func (c *client) IsOnline(ctx context.Context, uid string) (ok bool, err error) {
	if !c.IsReady() {
		return false, ErrClientNotReady
	}
	resp, err := c.rpc.IsOnline(ctx, &pb.UserReq{
		Uid: uid,
	})
	if err != nil {
		return false, err
	}
	return resp.Ok, nil
}
func (c *client) KickUser(ctx context.Context, uid string) (err error) {
	if !c.IsReady() {
		return ErrClientNotReady
	}
	_, err = c.rpc.KickUser(ctx, &pb.UserReq{
		Uid: uid,
	})
	return err
}
func (c *client) SendToAll(ctx context.Context, qos packet.MessageQos, kind packet.MessageKind, message []byte) (total, success int32, err error) {
	if !c.IsReady() {
		return 0, 0, ErrClientNotReady
	}
	resp, err := c.rpc.SendToAll(ctx, &pb.ToAllReq{
		Qos:     int32(qos),
		Kind:    int32(kind),
		Message: message,
	})
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}
func (c *client) SendToUser(ctx context.Context, uid string, qos packet.MessageQos, kind packet.MessageKind, message []byte) (total, success int32, err error) {
	if !c.IsReady() {
		return 0, 0, ErrClientNotReady
	}
	resp, err := c.rpc.SendToUser(ctx, &pb.ToUserReq{
		Uid:     uid,
		Qos:     int32(qos),
		Kind:    int32(kind),
		Message: message,
	})
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}
func (c *client) SendToChannel(ctx context.Context, channel string, qos packet.MessageQos, kind packet.MessageKind, message []byte) (total, success int32, err error) {
	if !c.IsReady() {
		return 0, 0, ErrClientNotReady
	}
	resp, err := c.rpc.SendToChannel(ctx, &pb.ToChannelReq{
		Channel: channel,
		Qos:     int32(qos),
		Kind:    int32(kind),
		Message: message,
	})
	if err != nil {
		return 0, 0, err
	}
	return resp.Total, resp.Success, nil
}
func (c *client) JoinChannel(ctx context.Context, uid string, channels map[string]string) (err error) {
	if !c.IsReady() {
		return ErrClientNotReady
	}
	_, err = c.rpc.JoinChannel(ctx, &pb.JoinReq{
		Uid:      uid,
		Channels: channels,
	})
	return err
}
func (c *client) LeaveChannel(ctx context.Context, uid string, channels map[string]string) (err error) {
	if !c.IsReady() {
		return ErrClientNotReady
	}
	_, err = c.rpc.LeaveChannel(ctx, &pb.JoinReq{
		Uid:      uid,
		Channels: channels,
	})
	return err
}
func (c *client) GetChannels(ctx context.Context, uid string) (channels map[string]string, err error) {
	if !c.IsReady() {
		return nil, ErrClientNotReady
	}
	resp, err := c.rpc.GetChannels(ctx, &pb.UserReq{
		Uid: uid,
	})
	if err != nil {
		return nil, err
	}
	return resp.Channels, nil
}
