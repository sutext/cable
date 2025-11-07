package broker

import (
	"fmt"
	"strings"
	"sync"

	"sutext.github.io/cable"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Broker interface {
	SendMessage(m *packet.Message) error
	JoinChannel(channels []string, id *packet.Identity) error
	LeaveChannel(channels []string, id *packet.Identity) error
}

type broker struct {
	channels   sync.Map
	taskQueue  *queue.Queue
	listeners  []server.Server
	peerServer server.Server
	peers      []*peer
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{}
	b.taskQueue = queue.NewQueue(options.queueWorker, options.queueBuffer)
	b.peers = make([]*peer, len(options.peers))
	for idx, p := range options.peers {
		strs := strings.Split(p, "@")
		if len(strs) != 2 {
			panic("invalid peer address")
		}
		b.peers[idx] = newPeer(strs[0], strs[1])
	}
	b.listeners = make([]server.Server, len(options.listeners))
	for idx, l := range options.listeners {
		b.listeners[idx] = cable.NewServer(l.Address, server.WithConnect(b.onUserConnect))
	}
	b.peerServer = cable.NewServer(options.peerListener.Address, server.WithRequest(b.onPeerRequest))
	return b
}
func (b *broker) Start() error {
	for _, l := range b.listeners {
		go l.Serve()
	}
	for _, p := range b.peers {
		p.Connect()
	}
	go b.peerServer.Serve()
	return nil
}

func (b *broker) SendMessage(m *packet.Message) error {
	if err := b.sendMessage(m); err != nil {
		return err
	}
	for _, p := range b.peers {
		if err := p.SendMessage(m); err != nil {
			return err
		}
	}
	return nil
}
func (b *broker) JoinChannel(channels []string, id *packet.Identity) error {
	for _, c := range channels {
		v, _ := b.channels.LoadOrStore(c, &sync.Map{})
		set := v.(*sync.Map)
		set.Store(id.ClientID, true)
	}
	return nil
}

func (b *broker) LeaveChannel(channels []string, id *packet.Identity) error {
	for _, c := range channels {
		if set, ok := b.channels.Load(c); ok {
			set.(*sync.Map).Delete(id.ClientID)
		}
	}
	return nil
}
func (b *broker) onUserConnect(p *packet.Connect) packet.ConnackCode {
	b.JoinChannel([]string{p.Identity.UserID}, p.Identity)
	return packet.ConnectionAccepted
}
func (b *broker) onPeerRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	switch p.Method {
	case "SendMessage":
		msg := &packet.Message{}
		if err := packet.Unmarshal(msg, p.Body); err != nil {
			return nil, err
		}
		if err := b.sendMessage(msg); err != nil {
			return nil, err
		}
		return &packet.Response{
			Serial:  p.Serial,
			Headers: map[string]string{},
			Body:    []byte{},
		}, nil
	case "KickConn":
		b.kickConnTo(string(p.Body))
		return &packet.Response{
			Serial:  p.Serial,
			Headers: map[string]string{},
			Body:    []byte{},
		}, nil
	}
	return nil, fmt.Errorf("unsupported request method: %s", p.Method)
}

func (b *broker) sendMessage(m *packet.Message) error {
	if set, ok := b.channels.Load(m.Channel); ok {
		set.(*sync.Map).Range(func(key, value any) bool {
			b.taskQueue.Push(func() {
				b.sendMessageTo(key.(string), m)
			})
			return true
		})
	}
	return nil
}
func (b *broker) kickConnTo(cid string) {
	for _, l := range b.listeners {
		l.KickConn(cid)
	}
}
func (b *broker) sendMessageTo(cid string, m *packet.Message) {
	for _, l := range b.listeners {
		if conn, err := l.GetConn(cid); err != nil {
			conn.SendMessage(m)
		}
	}
}
