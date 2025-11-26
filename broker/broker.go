package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/keymap"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/internal/muticast"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type BrokerID string
type Broker interface {
	Start() error
	Shutdown(ctx context.Context) error
	Inspects() ([]*Inspect, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickConn(ctx context.Context, cid string)
	KickUser(ctx context.Context, uid string)
	SendMessage(ctx context.Context, m *packet.Message) (total, success uint64, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
	LeaveChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
}
type broker struct {
	id             string
	peers          safe.Map[string, *peer]
	logger         logger.Logger
	handler        Handler
	muticast       muticast.Muticast
	listeners      map[server.Network]server.Server
	peerServer     server.Server
	httpServer     *http.Server
	brokerCount    atomic.Uint32
	userClients    keymap.KeyMap[server.Network] //map[uid]map[cid]net
	channelClients keymap.KeyMap[server.Network] //map[channel]map[cid]net
	clientChannels keymap.KeyMap[server.Network] //map[cid]map[channel]net
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.brokerCount.Add(1)
	b.logger = options.logger
	b.handler = options.handler
	b.muticast = muticast.New(b.id)
	b.muticast.OnRequest(func(s string) uint32 {
		b.addPeer(s)
		return b.brokerCount.Load()
	})
	for _, p := range options.peers {
		b.addPeer(p)
	}
	b.listeners = make(map[server.Network]server.Server, len(options.listeners))
	for net, addr := range options.listeners {
		b.listeners[net] = server.New(addr,
			server.WithClose(b.onUserClosed),
			server.WithConnect(func(p *packet.Connect) packet.ConnectCode {
				return b.onUserConnect(p, net)
			}),
			server.WithMessage(b.onUserMessage),
			server.WithRequest(b.onUserRequest),
			server.WithLogger(options.logger),
		)
	}
	b.peerServer = server.New(strings.Split(b.id, "@")[1], server.WithRequest(b.onPeerRequest))
	mux := http.NewServeMux()
	mux.HandleFunc("/join", b.handleJoin)
	mux.HandleFunc("/inspect", b.handleInspect)
	mux.HandleFunc("/kickout", b.handleKickout)
	mux.HandleFunc("/message", b.handleMessage)
	b.httpServer = &http.Server{Addr: options.httpAddr, Handler: mux}
	return b
}
func (b *broker) addPeer(id string) {
	if id == b.id {
		return
	}
	strs := strings.Split(id, "@")
	if len(strs) != 2 {
		return
	}
	if _, ok := b.peers.Get(id); ok {
		return
	}
	b.logger.Info("add peer", "id", id, "self", b.id)
	peer := newPeer(b.id, strs[1], b.logger)
	b.peers.Set(id, peer)
	b.brokerCount.Add(1)
	go peer.Connect()
}
func (b *broker) Start() error {
	for _, l := range b.listeners {
		go l.Serve()
	}
	go b.muticast.Serve()
	go b.peerServer.Serve()
	go b.httpServer.ListenAndServe()
	time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.syncBroker)
	return nil
}
func (b *broker) syncBroker() {
	if m, err := b.muticast.Request(); err != nil {
		var max uint32 = 0
		var min uint32 = 0xffff
		for id, count := range m {
			if count > max {
				max = count
			}
			if count < min {
				min = count
			}
			b.addPeer(id)
		}
		count := b.brokerCount.Load()
		if count > max {
			max = count
		}
		if count < min {
			min = count
		}
		if max != min {
			b.logger.Info("sync broker", "count", count, "max", max, "min", min)
			b.syncBroker()
		}
	}
}
func (b *broker) Shutdown(ctx context.Context) error {
	for _, l := range b.listeners {
		l.Shutdown(ctx)
	}
	return b.peerServer.Shutdown(ctx)
}

func (b *broker) IsOnline(ctx context.Context, uid string) (ok bool) {
	if ok = b.isOnline(uid); ok {
		return true
	}
	b.peers.Range(func(id string, peer *peer) bool {
		if ok, _ = peer.isOnline(ctx, uid); ok {
			ok = true
			return false
		}
		return true
	})
	return ok
}
func (b *broker) KickConn(ctx context.Context, cid string) {
	b.kickConn(cid)
	b.peers.Range(func(id string, peer *peer) bool {
		peer.kickConn(ctx, cid)
		return true
	})
}
func (b *broker) KickUser(ctx context.Context, uid string) {
	b.kickUser(uid)
	b.peers.Range(func(id string, peer *peer) bool {
		peer.kickUser(ctx, uid)
		return true
	})
}
func (b *broker) SendMessage(ctx context.Context, m *packet.Message) (total, success uint64, err error) {
	total, success = b.sendMessage(m)
	b.peers.Range(func(id string, peer *peer) bool {
		if t, s, err := peer.sendMessage(ctx, m); err == nil {
			total += t
			success += s
		} else {
			return false
		}
		return true
	})
	return total, success, nil
}
func (b *broker) JoinChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error) {
	count = b.joinChannel(uid, channels)
	b.peers.Range(func(id string, peer *peer) bool {
		if c, err := peer.joinChannel(ctx, uid, channels); err == nil {
			count += c
		}
		return true
	})
	return count, nil
}
func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error) {
	count = b.leaveChannel(uid, channels)
	b.peers.Range(func(id string, peer *peer) bool {
		if c, err := peer.leaveChannel(ctx, uid, channels); err == nil {
			count += c
		}
		return true
	})
	return count, nil
}

func (b *broker) onUserClosed(id *packet.Identity) {
	b.userClients.DeleteKey(id.UserID, id.ClientID)
	b.channelClients.DeleteKey("all", id.ClientID)
	b.clientChannels.RangeKey(id.ClientID, func(channel string, net server.Network) bool {
		b.channelClients.DeleteKey(channel, id.ClientID)
		return true
	})
	b.clientChannels.Delete(id.ClientID)
	b.handler.OnClosed(id)
}
func (b *broker) onUserConnect(p *packet.Connect, net server.Network) packet.ConnectCode {
	code := b.handler.OnConnect(p)
	if code != packet.ConnectAccepted {
		return code
	}
	b.peers.Range(func(id string, peer *peer) bool {
		peer.kickConn(context.Background(), p.Identity.ClientID)
		return true
	})
	b.userClients.SetKey(p.Identity.UserID, p.Identity.ClientID, net)
	b.channelClients.SetKey("all", p.Identity.ClientID, net)
	if chs, err := b.handler.GetChannels(p.Identity.UserID); err == nil {
		for _, ch := range chs {
			b.clientChannels.SetKey(p.Identity.ClientID, ch, net)
			b.channelClients.SetKey(ch, p.Identity.ClientID, net)
		}
	}
	return packet.ConnectAccepted
}
func (b *broker) onUserMessage(p *packet.Message, id *packet.Identity) error {
	return b.handler.OnMessage(p, id)
}
func (b *broker) onUserRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	return b.handler.OnRequest(p, id)
}
func (b *broker) onPeerRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	switch p.Method {
	case "SendMessage":
		msg := &packet.Message{}
		if err := coder.Unmarshal(p.Content, msg); err != nil {
			return nil, err
		}
		total, success := b.sendMessage(msg)
		encoder := coder.NewEncoder()
		encoder.WriteVarint(total)
		encoder.WriteVarint(success)
		return packet.NewResponse(p.ID, encoder.Bytes()), nil
	case "IsOnline":
		online := b.isOnline(string(p.Content))
		var r byte
		if online {
			r = 1
		}
		return packet.NewResponse(p.ID, []byte{r}), nil
	case "KickConn":
		b.kickConn(string(p.Content))
		return packet.NewResponse(p.ID), nil
	case "KickUser":
		b.kickUser(string(p.Content))
		return packet.NewResponse(p.ID), nil
	case "Inspect":
		isp := b.inspect()
		data, err := json.Marshal(isp)
		if err != nil {
			return nil, err
		}
		return packet.NewResponse(p.ID, data), nil
	case "JoinChannel":
		decoder := coder.NewDecoder(p.Content)
		uid, err := decoder.ReadString()
		if err != nil {
			return nil, err
		}
		chs, err := decoder.ReadStrings()
		if err != nil {
			return nil, err
		}
		count := b.joinChannel(uid, chs)
		encoder := coder.NewEncoder()
		encoder.WriteVarint(count)
		return packet.NewResponse(p.ID, encoder.Bytes()), nil
	case "LeaveChannel":
		decoder := coder.NewDecoder(p.Content)
		uid, err := decoder.ReadString()
		if err != nil {
			return nil, err
		}
		chs, err := decoder.ReadStrings()
		if err != nil {
			return nil, err
		}
		count := b.leaveChannel(uid, chs)
		encoder := coder.NewEncoder()
		encoder.WriteVarint(count)
		return packet.NewResponse(p.ID, encoder.Bytes()), nil
	default:
		return nil, fmt.Errorf("unsupported request path: %s", p.Method)
	}
}
func (b *broker) isOnline(uid string) (ok bool) {
	b.userClients.RangeKey(uid, func(cid string, net server.Network) bool {
		if b.isActive(cid, net) {
			ok = true
			return false
		}
		return true
	})
	return ok
}
func (b *broker) isActive(cid string, net server.Network) (ok bool) {
	if l, ok := b.listeners[net]; ok {
		return l.IsActive(cid)
	}
	return false
}
func (b *broker) kickConn(cid string) {
	for _, l := range b.listeners {
		l.KickConn(cid)
	}
}
func (b *broker) kickUser(uid string) {
	b.userClients.RangeKey(uid, func(cid string, net server.Network) bool {
		if l, ok := b.listeners[net]; ok {
			l.KickConn(cid)
		}
		return true
	})
}
func (b *broker) sendMessage(m *packet.Message) (total, success uint64) {
	b.channelClients.RangeKey(m.Channel, func(cid string, net server.Network) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			b.logger.Error("Failed to find listener", "network", net)
			return true
		}
		if err := l.SendMessage(cid, m); err != nil {
			b.logger.Error("Failed to send message to client", "error", err, "client", cid)
		} else {
			success++
		}
		return true
	})
	return total, success
}

func (b *broker) joinChannel(uid string, channels []string) (count uint64) {
	b.userClients.RangeKey(uid, func(cid string, net server.Network) bool {
		count++
		for _, ch := range channels {
			b.channelClients.SetKey(ch, cid, net)
			b.clientChannels.SetKey(cid, ch, net)
		}
		return true
	})
	return count
}
func (b *broker) leaveChannel(uid string, channels []string) (count uint64) {
	b.userClients.RangeKey(uid, func(cid string, net server.Network) bool {
		count++
		for _, ch := range channels {
			b.channelClients.DeleteKey(ch, cid)
			b.clientChannels.DeleteKey(cid, ch)
		}
		return true
	})
	return count
}
