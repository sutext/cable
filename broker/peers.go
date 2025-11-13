package broker

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

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
func (p *peer) IsReady() bool {
	return p.client.Status() == client.StatusOpened
}
func (p *peer) Connect() {
	p.client.Connect(p.id)
}
func (p *peer) sendMessage(ctx context.Context, m *packet.Message) (total, success uint64, err error) {
	body, err := packet.Marshal(m)
	if err != nil {
		return
	}
	req := packet.NewRequest("SendMessage", body)
	res, err := p.client.Request(ctx, req)
	if err != nil {
		return
	}
	decoder := packet.NewDecoder(res.Body)
	if total, err = decoder.ReadVarint(); err != nil {
		return 0, 0, err
	}
	if success, err = decoder.ReadVarint(); err != nil {
		return 0, 0, err
	}
	return
}
func (p *peer) isOnline(ctx context.Context, uid string) (bool, error) {
	req := packet.NewRequest("IsOnline", []byte(uid))
	res, err := p.client.Request(ctx, req)
	if err != nil {
		return false, err
	}
	return res.Body[0] == 1, nil
}
func (p *peer) kickConn(ctx context.Context, cid string) error {
	req := packet.NewRequest("KickConn", []byte(cid))
	_, err := p.client.Request(ctx, req)
	return err
}
func (p *peer) kickUser(ctx context.Context, uid string) error {
	req := packet.NewRequest("KickUser", []byte(uid))
	_, err := p.client.Request(ctx, req)
	return err
}
func (p *peer) inspect(ctx context.Context) (*Inspect, error) {
	req := packet.NewRequest("Inspect", nil)
	res, err := p.client.Request(ctx, req)
	if err != nil {
		return nil, err
	}
	var isp Inspect
	if err := json.Unmarshal(res.Body, &isp); err != nil {
		return nil, err
	}
	return &isp, nil
}
