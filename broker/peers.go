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
	id     string
	client client.Client
}

func newPeer(id, address string, logger logger.Logger) *peer {
	p := &peer{}
	p.id = id
	p.client = client.New(address,
		client.WithLogger(logger),
		client.WithRetry(math.MaxInt, backoff.Constant(time.Second*5)),
		client.WithKeepAlive(time.Second*3, time.Second*60),
		client.WithRequestTimeout(time.Second*3),
	)
	return p
}
func (p *peer) ID() string {
	return p.ID()
}
func (p *peer) IsReady() bool {
	return p.client.Status() == client.StatusOpened
}
func (p *peer) Connect() {
	p.client.Connect(&packet.Identity{ClientID: p.id, UserID: p.id})
}
func (p *peer) sendMessage(ctx context.Context, m *packet.Message) (total, success uint64, err error) {
	body, err := packet.Marshal(m)
	if err != nil {
		return
	}
	req := packet.NewRequest("SendMessage", body)
	res, err := p.client.SendRequest(ctx, req)
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
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return false, err
	}
	return res.Body[0] == 1, nil
}
func (p *peer) kickConn(ctx context.Context, cid string) error {
	req := packet.NewRequest("KickConn", []byte(cid))
	_, err := p.client.SendRequest(ctx, req)
	return err
}
func (p *peer) kickUser(ctx context.Context, uid string) error {
	req := packet.NewRequest("KickUser", []byte(uid))
	_, err := p.client.SendRequest(ctx, req)
	return err
}
func (p *peer) inspect(ctx context.Context) (*Inspect, error) {
	req := packet.NewRequest("Inspect")
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	var isp Inspect
	if err := json.Unmarshal(res.Body, &isp); err != nil {
		return nil, err
	}
	return &isp, nil
}

func (p *peer) joinChannel(ctx context.Context, uid string, channels []string) (count uint64, err error) {
	encoder := packet.NewEncoder()
	encoder.WriteString(uid)
	encoder.WriteStrings(channels)
	req := packet.NewRequest("JoinChannel", encoder.Bytes())
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	decoder := packet.NewDecoder(res.Body)
	if count, err = decoder.ReadVarint(); err != nil {
		return 0, err
	}
	return count, nil
}

func (p *peer) leaveChannel(ctx context.Context, uid string, channels []string) (count uint64, err error) {
	encoder := packet.NewEncoder()
	encoder.WriteString(uid)
	encoder.WriteStrings(channels)
	req := packet.NewRequest("LeaveChannel", encoder.Bytes())
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	decoder := packet.NewDecoder(res.Body)
	if count, err = decoder.ReadVarint(); err != nil {
		return 0, err
	}
	return count, nil
}
func (p *peer) joinCluster(ctx context.Context, brokerID BrokerID) error {
	req := packet.NewRequest("JoinCluster", []byte(brokerID))
	_, err := p.client.SendRequest(ctx, req)
	return err
}
