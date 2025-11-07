package broker

import (
	"context"
	"math"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/packet"
)

type Peer interface {
	SendMessage(m *packet.Message) error
	KickConn(cid string) error
}

type peer struct {
	id     *packet.Identity
	client client.Client
}

func newPeer(id, address string) *peer {
	p := &peer{}
	p.id = &packet.Identity{
		UserID:   id,
		ClientID: id,
	}
	p.client = client.New(address, client.WithRetry(math.MaxInt, backoff.Constant(time.Second*5)))
	return p
}
func (p *peer) ID() *packet.Identity {
	return p.id
}

func (p *peer) Connect() {
	p.client.Connect(p.id)
}
func (p *peer) SendMessage(m *packet.Message) error {
	body, err := packet.Marshal(m)
	if err != nil {
		return err
	}
	req := &packet.Request{
		Method: "SendMessage",
		Body:   body,
	}
	_, err = p.client.Request(context.Background(), req)
	return err
}

func (p *peer) KickConn(cid string) error {
	req := &packet.Request{
		Method: "KickConn",
		Body:   []byte(cid),
	}
	_, err := p.client.Request(context.Background(), req)
	return err
}
