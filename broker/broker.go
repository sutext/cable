package broker

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"sutext.github.io/cable"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Broker interface {
	Start() error
	Shutdown() error
	IsOnline(ctx context.Context, uid string) (ok bool)
	SendMessage(ctx context.Context, m *packet.Message) error
	JoinChannel(ctx context.Context, channels []string, id *packet.Identity) error
	LeaveChannel(ctx context.Context, channels []string, id *packet.Identity) error
}
type broker struct {
	id          string
	peers       []*peer
	logger      logger.Logger
	channels    sync.Map
	taskQueue   *queue.Queue
	listeners   []server.Server
	peerServer  server.Server
	userClients sync.Map
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.logger = options.logger
	b.taskQueue = queue.NewQueue(options.queueWorker, options.queueBuffer)
	for _, p := range options.peers {
		if p == b.id {
			continue
		}
		strs := strings.Split(p, "@")
		if len(strs) != 2 {
			panic("invalid peer address")
		}
		b.peers = append(b.peers, newPeer(b.id, strs[1], options.logger))
	}
	b.listeners = make([]server.Server, len(options.listeners))
	for idx, l := range options.listeners {
		b.listeners[idx] = cable.NewServer(l.Address,
			server.WithConnect(b.onUserConnect),
			server.WithMessage(b.onUserMessage),
			server.WithLogger(options.logger),
		)
	}
	b.peerServer = cable.NewServer(strings.Split(b.id, "@")[1], server.WithRequest(b.onPeerRequest))
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
func (b *broker) Shutdown() error {
	ctx := context.Background()
	for _, l := range b.listeners {
		l.Shutdown(ctx)
	}
	return b.peerServer.Shutdown(ctx)
}
func (b *broker) IsOnline(ctx context.Context, uid string) (ok bool) {
	if ok = b.isOnline(uid); ok {
		return true
	}
	for _, p := range b.peers {
		if ok, _ = p.IsOnline(ctx, uid); ok {
			return true
		}
	}
	return false
}

func (b *broker) SendMessage(ctx context.Context, m *packet.Message) error {
	if err := b.sendMessage(m); err != nil {
		return err
	}
	for _, p := range b.peers {
		if err := p.SendMessage(ctx, m); err != nil {
			return err
		}
	}
	b.logger.Info("Sent message to all peers", "peers", len(b.peers), "channel", m.Channel, "broker", b.id)
	return nil
}
func (b *broker) JoinChannel(ctx context.Context, channels []string, id *packet.Identity) error {
	for _, c := range channels {
		v, _ := b.channels.LoadOrStore(c, &sync.Map{})
		set := v.(*sync.Map)
		set.Store(id.ClientID, true)
	}
	return nil
}

func (b *broker) LeaveChannel(ctx context.Context, channels []string, id *packet.Identity) error {
	for _, c := range channels {
		if set, ok := b.channels.Load(c); ok {
			set.(*sync.Map).Delete(id.ClientID)
		}
	}
	return nil
}
func (b *broker) onUserMessage(p *packet.Message, id *packet.Identity) error {
	err := b.SendMessage(context.Background(), p)
	if err != nil {
		b.logger.Error("Failed to send message to broker", "error", err, "broker", b.id)
	}
	return err
}
func (b *broker) onUserConnect(p *packet.Connect) packet.ConnackCode {
	v, _ := b.userClients.LoadOrStore(p.Identity.UserID, &sync.Map{})
	v.(*sync.Map).Store(p.Identity.ClientID, true)
	b.JoinChannel(context.Background(), []string{"news", "sports", "tech", "music", "movies"}, p.Identity)
	return packet.ConnectionAccepted
}
func (b *broker) onPeerRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	switch p.Path {
	case "SendMessage":
		msg := &packet.Message{}
		if err := packet.Unmarshal(msg, p.Body); err != nil {
			return nil, err
		}
		if err := b.sendMessage(msg); err != nil {
			return nil, err
		}
		b.logger.Info("Received message from peer", "channel", msg.Channel, "broker", b.id)
		return packet.NewResponse(p.Seq), nil
	case "IsOnline":
		online := b.isOnline(string(p.Body))
		var r byte
		if online {
			r = 1
		}
		return packet.NewResponse(p.Seq, []byte{r}), nil
	case "KickConn":
		b.kickConn(string(p.Body))
		return packet.NewResponse(p.Seq), nil
	case "KickUser":
		b.kickUser(string(p.Body))
		return packet.NewResponse(p.Seq), nil
	}
	return nil, fmt.Errorf("unsupported request method: %s", p.Path)
}
func (b *broker) isOnline(uid string) (ok bool) {
	v, ok := b.userClients.Load(uid)
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
	if set, ok := b.userClients.Load(uid); ok {
		set.(*sync.Map).Range(func(key, value any) bool {
			b.kickConn(key.(string))
			return true
		})
	}
}
func (b *broker) sendMessage(m *packet.Message) error {
	if set, ok := b.channels.Load(m.Channel); ok {
		set.(*sync.Map).Range(func(key, value any) bool {
			cid := key.(string)
			b.taskQueue.Push(func() {
				for _, l := range b.listeners {
					if conn, err := l.GetConn(cid); err != nil {
						b.logger.Error("Failed to get client connection", "error", err, "client", cid)
						if err = conn.SendMessage(m); err != nil {
							b.logger.Error("Failed to send message to client", "error", err, "client", cid)
						}
					}
				}
			})
			return true
		})
	}
	return nil
}
