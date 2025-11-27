package broker

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/keymap"
	"sutext.github.io/cable/internal/muticast"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type BrokerID string
type Broker interface {
	Start() error
	Shutdown(ctx context.Context) error
	Inspects() ([]*Inspect, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickConn(ctx context.Context, cid string)
	KickUser(ctx context.Context, uid string)
	Brodcast(ctx context.Context, m *packet.Message) (total, success uint64, err error)
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success uint64, err error)
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success uint64, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
	LeaveChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
	HandleRequest(method string, handler server.RequestHandler)
}
type broker struct {
	id             string
	peers          safe.Map[string, *peer]
	logger         *xlog.Logger
	handler        Handler
	muticast       muticast.Muticast
	listeners      map[server.Network]server.Server
	peerServer     server.Server
	httpServer     *http.Server
	brokerCount    atomic.Uint32
	userClients    keymap.KeyMap[server.Network] //map[uid]map[cid]net
	channelClients keymap.KeyMap[server.Network] //map[channel]map[cid]net
	clientChannels keymap.KeyMap[server.Network] //map[cid]map[channel]net
	peerHandlers   safe.Map[string, server.RequestHandler]
	userHandlers   safe.Map[string, server.RequestHandler]
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.brokerCount.Add(1)
	b.logger = xlog.With("GROUP", "BROKER", "selfid", b.id)
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
		)
	}
	b.peerServer = server.New(fmt.Sprintf(":%s", strings.Split(b.id, ":")[1]), server.WithRequest(b.onPeerRequest))
	b.handlePeerRequest("SendMessage", b.handleSendMessage)
	b.handlePeerRequest("IsOnline", b.handleIsOnline)
	b.handlePeerRequest("KickConn", b.handleKickConn)
	b.handlePeerRequest("JoinChannel", b.handleJoinChannel)
	b.handlePeerRequest("LeaveChannel", b.handleLeaveChannel)
	b.handlePeerRequest("Inspect", b.handlePeerInspect)
	b.handlePeerRequest("KickUser", b.handleKickUser)
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
	b.logger.Debug("add peer", slog.String("peerid", id))
	peer := newPeer(b.id, strs[1])
	b.peers.Set(id, peer)
	b.brokerCount.Add(1)
	go peer.Connect()
}
func (b *broker) Start() error {
	for _, l := range b.listeners {
		go func() {
			if err := l.Serve(); err != nil {
				b.logger.Error("listener serve", err)
			}
		}()
	}
	go func() {
		if err := b.muticast.Serve(); err != nil {
			b.logger.Error("muticast serve ", err)
		}
	}()
	go func() {
		if err := b.peerServer.Serve(); err != nil {
			b.logger.Error("peer server serve", err)
		}
	}()
	go func() {
		if err := b.httpServer.ListenAndServe(); err != nil {
			b.logger.Error("http server serve", err)
		}
	}()
	time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.syncBroker)
	return nil
}

func (b *broker) syncBroker() {
	m, err := b.muticast.Request()
	if err != nil {
		b.logger.Error("failed to sync broker", err)
	}
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
		b.logger.Warn("broker count mismatch", slog.Uint64("max", uint64(max)), slog.Uint64("min", uint64(min)))
		b.syncBroker()
	}
}
func (b *broker) Shutdown(ctx context.Context) error {
	for _, l := range b.listeners {
		l.Shutdown(ctx)
	}
	b.muticast.Shutdown()
	b.httpServer.Shutdown(ctx)
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
func (b *broker) Brodcast(ctx context.Context, m *packet.Message) (total, success uint64, err error) {
	total, success = b.brodcast(m)
	b.peers.Range(func(id string, peer *peer) bool {
		if t, s, err := peer.sendMessage(ctx, m, "", 0); err == nil {
			total += t
			success += s
		} else {
			return false
		}
		return true
	})
	return total, success, nil
}
func (b *broker) SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success uint64, err error) {
	if uid == "" {
		return 0, 0, ErrInvalidUserID
	}
	total, success = b.sendToUser(uid, m)
	b.peers.Range(func(id string, peer *peer) bool {
		if t, s, err := peer.sendMessage(ctx, m, uid, 1); err == nil {
			total += t
			success += s
		} else {
			return false
		}
		return true
	})
	return total, success, nil
}
func (b *broker) SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success uint64, err error) {
	if channel == "" {
		return 0, 0, ErrInvalidChannel
	}
	total, success = b.sendToChannel(channel, m)
	b.peers.Range(func(id string, peer *peer) bool {
		if t, s, err := peer.sendMessage(ctx, m, channel, 2); err == nil {
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
	b.clientChannels.RangeKey(id.ClientID, func(channel string, net server.Network) bool {
		b.channelClients.DeleteKey(channel, id.ClientID)
		return true
	})
	b.clientChannels.Delete(id.ClientID)
	b.handler.OnClosed(id)
}
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.userHandlers.Set(method, handler)
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
	handler, ok := b.userHandlers.Get(p.Method)
	if !ok {
		return nil, ErrRequestHandlerNotFound
	}
	return handler(p, id)
}
func (b *broker) handlePeerRequest(method string, handler server.RequestHandler) {
	b.peerHandlers.Set(method, handler)
}
func (b *broker) onPeerRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	handler, ok := b.peerHandlers.Get(p.Method)
	if !ok {
		return nil, ErrRequestHandlerNotFound
	}
	return handler(p, id)
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
func (b *broker) brodcast(m *packet.Message) (total, success uint64) {
	for _, l := range b.listeners {
		t, s, err := l.Brodcast(m)
		total += t
		success += s
		if err != nil {
			b.logger.Error("Failed to send message to all clients", err)
		}
	}
	return total, success
}
func (b *broker) sendToUser(uid string, m *packet.Message) (total, success uint64) {
	b.userClients.RangeKey(uid, func(cid string, net server.Network) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			return true
		}
		if err := l.SendMessage(cid, m); err != nil {
			b.logger.Error("Failed to send message to client", err)
		} else {
			success++
		}
		return true
	})
	return total, success
}
func (b *broker) sendToChannel(channel string, m *packet.Message) (total, success uint64) {
	b.channelClients.RangeKey(channel, func(cid string, net server.Network) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			return true
		}
		if err := l.SendMessage(cid, m); err != nil {
			b.logger.Error("Failed to send message to client", err)
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
