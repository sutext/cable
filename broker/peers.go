package broker

import (
	"context"

	"sutext.github.io/cable/api/kitex_gen/api"
	"sutext.github.io/cable/api/kitex_gen/api/service"
	"sutext.github.io/cable/client"
)

type peerClient struct {
	client service.Client
}

func NewPeer(client *client.Client) *peerClient {
	return &peerClient{}
}
func (p *peerClient) Join(ctx context.Context, channels []string, uid string) error {
	_, err := p.client.Join(ctx, &api.JoinRequest{Uid: uid, Channels: channels})
	return err
}
func (p *peerClient) Leave(ctx context.Context, channels []string, uid string) error {
	_, err := p.client.Leave(ctx, &api.JoinRequest{Uid: uid, Channels: channels})
	return err
}
func (p *peerClient) Publish(ctx context.Context, channel string, payload []byte) error {
	return p.client.Publish(ctx, &api.PublishRequest{Channel: channel, Payload: payload})
}

// PeersImpl implements the last service interface defined in the IDL.
type peerServer struct {
	broker *broker
}

// Join implements the PeersImpl interface.
func (s *peerServer) Join(ctx context.Context, req *api.JoinRequest) (resp *api.JoinResponse, err error) {
	// TODO: Your code here...
	s.broker.join(req.Channels, req.Uid)
	return api.NewJoinResponse(), nil

}

// Leave implements the PeersImpl interface.
func (s *peerServer) Leave(ctx context.Context, req *api.JoinRequest) (resp *api.JoinResponse, err error) {
	// TODO: Your code here...
	s.broker.leave(req.Channels, req.Uid)
	return api.NewJoinResponse(), nil
}

// SendData implements the PeersImpl interface.
func (s *peerServer) Publish(ctx context.Context, req *api.PublishRequest) error {
	// TODO: Your code here...
	s.broker.publish(req.Channel, req.Payload)
	return nil
}
