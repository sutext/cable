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
	IsOnline(uid string) bool
	SendMessage(m *packet.Message) error
	JoinChannel(channels []string, id *packet.Identity) error
	LeaveChannel(channels []string, id *packet.Identity) error
}

type broker struct {
	userChannles sync.Map
	channels     sync.Map
	taskQueue    *queue.Queue
	listeners    []server.Server
	peerServer   server.Server
	peers        []*peer
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
func (b *broker) IsOnline(uid string) (ok bool) {
	ok = b.isOnline(uid)
	if ok {
		return true
	}
	for _, p := range b.peers {
		ok, _ = p.IsOnline(uid)
		if ok {
			return true
		}
	}
	return false
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
	v, _ := b.userChannles.LoadOrStore(p.Identity.UserID, &sync.Map{})
	v.(*sync.Map).Store(p.Identity.ClientID, true)
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
			Serial: p.Serial,
			Body:   []byte{},
		}, nil
	case "IsOnline":
		online := b.isOnline(string(p.Body))
		var r byte
		if online {
			r = 1
		}
		return &packet.Response{
			Serial: p.Serial,
			Body:   []byte{r},
		}, nil
	case "KickConn":
		b.kickConn(string(p.Body))
		return &packet.Response{
			Serial: p.Serial,
			Body:   []byte{},
		}, nil
	case "KickUser":
		b.kickUser(string(p.Body))
		return &packet.Response{
			Serial:  p.Serial,
			Headers: map[string]string{},
			Body:    []byte{},
		}, nil
	}
	return nil, fmt.Errorf("unsupported request method: %s", p.Method)
}
func (b *broker) isOnline(uid string) (ok bool) {
	v, ok := b.userChannles.Load(uid)
	if !ok {
		return false
	}
	v.(*sync.Map).Range(func(key, value any) bool {
		cid := key.(string)
		for _, l := range b.listeners {
			if conn, err := l.GetConn(cid); err == nil {
				if conn.IsActive() {
					ok = true
					return false
				}
			}
		}
		return true
	})
	return ok
}
func (b *broker) kickConn(cid string) {
	for _, l := range b.listeners {
		l.KickConn(cid)
	}
}
func (b *broker) kickUser(uid string) {
	if set, ok := b.userChannles.Load(uid); ok {
		set.(*sync.Map).Range(func(key, value any) bool {
			b.kickConn(key.(string))
			return true
		})
	}
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
	if set, ok := b.userChannles.Load(m.Channel); ok {
		set.(*sync.Map).Range(func(key, value any) bool {
			b.taskQueue.Push(func() {
				b.sendMessageTo(key.(string), m)
			})
			return true
		})
	}
	return nil
}
func (b *broker) sendMessageTo(cid string, m *packet.Message) {
	for _, l := range b.listeners {
		if conn, err := l.GetConn(cid); err != nil {
			conn.SendMessage(m)
		}
	}
}
