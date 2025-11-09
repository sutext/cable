package broker

import (
	"context"
	"math"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Peer interface {
	SendMessage(m *packet.Message) error
	IsOnline(uid string) (bool, error)
	KickConn(cid string) error
	KickUser(uid string) error
}

type peer struct {
	id     *packet.Identity
	client client.Client
}

func newPeer(id, address string, logger logger.Logger) *peer {
	p := &peer{}
	p.id = &packet.Identity{
		UserID:   id,
		ClientID: id,
	}
	p.client = client.New(address,
		client.WithLogger(logger),
		client.WithRetry(math.MaxInt, backoff.Constant(time.Second*5)),
		client.WithKeepAlive(time.Second*3, time.Second*60),
		client.WithRequestTimeout(time.Second*3),
	)
	return p
}
func (p *peer) ID() *packet.Identity {
	return p.id
}

func (p *peer) Connect() {
	p.client.Connect(p.id)
}
func (p *peer) SendMessage(ctx context.Context, m *packet.Message) error {
	body, err := packet.Marshal(m)
	if err != nil {
		return err
	}
	req := packet.NewRequest("SendMessage", body)
	_, err = p.client.Request(ctx, req)
	return err
}
func (p *peer) IsOnline(ctx context.Context, uid string) (bool, error) {
	req := packet.NewRequest("IsOnline", []byte(uid))
	res, err := p.client.Request(ctx, req)
	if err != nil {
		return false, err
	}
	return res.Body[0] == 1, nil
}
func (p *peer) KickConn(ctx context.Context, cid string) error {
	req := packet.NewRequest("KickConn", []byte(cid))
	_, err := p.client.Request(ctx, req)
	return err
}
func (p *peer) KickUser(ctx context.Context, uid string) error {
	req := packet.NewRequest("KickUser", []byte(uid))
	_, err := p.client.Request(ctx, req)
	return err
}
