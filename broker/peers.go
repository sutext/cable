package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type peer struct {
	id     string
	ip     string
	ipmu   sync.Mutex
	broker *broker
	client client.Client
}

func newPeer(id, ip string, broker *broker) *peer {
	p := &peer{
		id:     id,
		ip:     ip,
		broker: broker,
	}
	p.client = p.createClient()
	return p
}

func (p *peer) IsReady() bool {
	return p.client.Status() == client.StatusOpened
}
func (p *peer) SetIP(ip string) {
	p.ipmu.Lock()
	defer p.ipmu.Unlock()
	if p.ip == ip {
		return
	}
	p.broker.logger.Warn("peer ip updated", xlog.String("peerid", p.id), xlog.String("ip", ip))
	p.ip = ip
	p.client = p.createClient()
	p.Connect()
}
func (p *peer) SendMessage(ctx context.Context, m *packet.Message, target string, flag uint8) (total, success uint64, err error) {
	return p.sendMessage(ctx, m, target, flag)
}
func (p *peer) IsOnline(ctx context.Context, uid string) (bool, error) {
	return p.isOnline(ctx, uid)
}
func (p *peer) Connect() {
	p.client.Connect(&packet.Identity{ClientID: p.broker.id, UserID: p.broker.id})
}
func (p *peer) OnStatus(status client.Status) {
	switch status {
	case client.StatusOpened:
		p.broker.logger.Info("peer connected", xlog.String("peerid", p.id))
	case client.StatusClosed:
		p.broker.delPeer(p.id)
	}
}

func (h *peer) OnMessage(p *packet.Message) error {
	return nil
}
func (h *peer) OnRequest(p *packet.Request) (*packet.Response, error) {
	return nil, fmt.Errorf("no implemention")
}
func (p *peer) sendMessage(ctx context.Context, m *packet.Message, target string, flag uint8) (total, success uint64, err error) {
	if p.client.Status() != client.StatusOpened {
		return 0, 0, xerr.BrokerPeerNotReady
	}
	enc := coder.NewEncoder()
	enc.WriteUInt8(flag)
	enc.WriteString(target)
	if err := m.WriteTo(enc); err != nil {
		return 0, 0, err
	}
	req := packet.NewRequest("SendMessage", enc.Bytes())
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return 0, 0, err
	}
	decoder := coder.NewDecoder(res.Content)
	if total, err = decoder.ReadVarint(); err != nil {
		return 0, 0, err
	}
	if success, err = decoder.ReadVarint(); err != nil {
		return 0, 0, err
	}
	return
}
func (p *peer) createClient() client.Client {
	retrier := client.NewRetrier(math.MaxInt, backoff.ConstantD())
	retrier.Filter(func(err error) bool {
		return strings.Contains(err.Error(), "no route")
	})
	return client.New(fmt.Sprintf("%s%s", p.ip, p.broker.peerPort),
		client.WithRetrier(retrier),
		client.WithHandler(p),
		client.WithKeepAlive(time.Second*3, time.Second*60),
		client.WithRequestTimeout(time.Second*3),
	)
}
func (p *peer) isOnline(ctx context.Context, uid string) (bool, error) {
	if p.client.Status() != client.StatusOpened {
		return false, xerr.BrokerPeerNotReady
	}
	req := packet.NewRequest("IsOnline", []byte(uid))
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return false, err
	}
	return res.Content[0] == 1, nil
}
func (p *peer) kickConn(ctx context.Context, cid string) error {
	if p.client.Status() != client.StatusOpened {
		return xerr.BrokerPeerNotReady
	}
	req := packet.NewRequest("KickConn", []byte(cid))
	_, err := p.client.SendRequest(ctx, req)
	return err
}
func (p *peer) kickUser(ctx context.Context, uid string) error {
	if p.client.Status() != client.StatusOpened {
		return xerr.BrokerPeerNotReady
	}
	req := packet.NewRequest("KickUser", []byte(uid))
	_, err := p.client.SendRequest(ctx, req)
	return err
}
func (p *peer) inspect(ctx context.Context) (*Inspect, error) {
	if p.client.Status() != client.StatusOpened {
		return nil, xerr.BrokerPeerNotReady
	}
	req := packet.NewRequest("Inspect")
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	var isp Inspect
	if err := json.Unmarshal(res.Content, &isp); err != nil {
		return nil, err
	}
	return &isp, nil
}

func (p *peer) joinChannel(ctx context.Context, uid string, channels []string) (count uint64, err error) {
	if p.client.Status() != client.StatusOpened {
		return 0, xerr.BrokerPeerNotReady
	}
	encoder := coder.NewEncoder()
	encoder.WriteString(uid)
	encoder.WriteStrings(channels)
	req := packet.NewRequest("JoinChannel", encoder.Bytes())
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	decoder := coder.NewDecoder(res.Content)
	if count, err = decoder.ReadVarint(); err != nil {
		return 0, err
	}
	return count, nil
}

func (p *peer) leaveChannel(ctx context.Context, uid string, channels []string) (count uint64, err error) {
	if p.client.Status() != client.StatusOpened {
		return 0, xerr.BrokerPeerNotReady
	}
	encoder := coder.NewEncoder()
	encoder.WriteString(uid)
	encoder.WriteStrings(channels)
	req := packet.NewRequest("LeaveChannel", encoder.Bytes())
	res, err := p.client.SendRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	decoder := coder.NewDecoder(res.Content)
	if count, err = decoder.ReadVarint(); err != nil {
		return 0, err
	}
	return count, nil
}
func (p *peer) freeMemory(ctx context.Context) {
	p.client.SendRequest(ctx, packet.NewRequest("FreeMemory"))
}
