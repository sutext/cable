package broker

import (
	"context"
	"math/rand/v2"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"sutext.github.io/cable/internal/muticast"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xerr"
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
	SendToAll(ctx context.Context, m *packet.Message) (total, success uint64, err error)
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success uint64, err error)
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success uint64, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
	LeaveChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
	HandleRequest(method string, handler server.RequestHandler)
}
type broker struct {
	id             string
	peers          safe.XMap[string, *peer]
	logger         *xlog.Logger
	handler        Handler
	muticast       muticast.Muticast
	listeners      map[server.Network]server.Server
	peerPort       string
	peerServer     server.Server
	httpServer     *http.Server
	userClients    safe.KeyMap[server.Network] //map[uid]map[cid]net
	channelClients safe.KeyMap[server.Network] //map[channel]map[cid]net
	clientChannels safe.KeyMap[server.Network] //map[cid]map[channel]net
	peerHandlers   safe.Map[string, server.RequestHandler]
	userHandlers   safe.Map[string, server.RequestHandler]
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.logger = xlog.With("GROUP", "BROKER")
	b.handler = options.handler
	b.muticast = muticast.New(b.id)
	b.muticast.OnRequest(func(idip string) int32 {
		strs := strings.Split(idip, ":")
		b.addPeer(strs[0], strs[1])
		return b.clusterSize()
	})
	for _, idip := range options.peers {
		strs := strings.Split(idip, ":")
		if len(strs) != 2 {
			panic("peers format error want id:ip")
		}
		b.addPeer(strs[0], strs[1])
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
	b.peerPort = options.peerPort
	b.peerServer = server.New(options.peerPort,
		server.WithRequest(b.onPeerRequest),
		server.WithLogger(xlog.With("GROUP", "PEER")),
	)
	b.handlePeer("SendMessage", b.handleSendMessage)
	b.handlePeer("IsOnline", b.handleIsOnline)
	b.handlePeer("KickConn", b.handleKickConn)
	b.handlePeer("JoinChannel", b.handleJoinChannel)
	b.handlePeer("LeaveChannel", b.handleLeaveChannel)
	b.handlePeer("Inspect", b.handlePeerInspect)
	b.handlePeer("KickUser", b.handleKickUser)
	b.handlePeer("FreeMemory", b.handlePeerFreeMemory)
	mux := http.NewServeMux()
	mux.HandleFunc("/join", b.handleJoin)
	mux.HandleFunc("/inspect", b.handleInspect)
	mux.HandleFunc("/kickout", b.handleKickout)
	mux.HandleFunc("/message", b.handleMessage)
	mux.HandleFunc("/brodcast", b.handleBrodcast)
	mux.HandleFunc("/health", b.handleHealth)
	mux.HandleFunc("/freeMemory", b.handleFreeMemory)
	b.httpServer = &http.Server{Addr: options.httpPort, Handler: mux}
	return b
}
func (b *broker) clusterSize() int32 {
	return b.peers.Len() + 1
}
func (b *broker) delPeer(id string) {
	b.logger.Debug("del peer", xlog.String("peerid", id))
	if _, ok := b.peers.Get(id); ok {
		b.peers.Delete(id)
	}
}
func (b *broker) addPeer(id, ip string) {
	if id == b.id {
		return
	}
	if p, ok := b.peers.Get(id); ok {
		p.SetIP(ip)
		return
	}
	peer := newPeer(id, ip, b)
	if peer == nil {
		return
	}
	b.logger.Debug("add peer", xlog.String("peerid", id))
	b.peers.Set(id, peer)
	go peer.Connect()
}
func (b *broker) Start() error {
	for _, l := range b.listeners {
		go func() {
			if err := l.Serve(); err != nil {
				panic(err)
			}
		}()
	}
	go func() {
		if err := b.muticast.Serve(); err != nil {
			panic(err)
		}
	}()
	go func() {
		if err := b.peerServer.Serve(); err != nil {
			panic(err)
		}
	}()
	go func() {
		if err := b.httpServer.ListenAndServe(); err != nil {
			panic(err)
		}
	}()
	time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.syncBroker)
	return nil
}

func (b *broker) syncBroker() {
	m, err := b.muticast.Request()
	if err != nil {
		b.logger.Error("failed to sync broker", xlog.Err(err))
	}
	if len(m) == 0 {
		b.logger.Warn("Auto discovery got empty endpoints")
		time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.syncBroker)
	}
	max := b.clusterSize()
	min := b.clusterSize()
	for idip, count := range m {
		if count > max {
			max = count
		}
		if count < min {
			min = count
		}
		strs := strings.Split(idip, ":")
		b.addPeer(strs[0], strs[1])
	}
	if max != min {
		b.logger.Warn("broker count mismatch", xlog.Int("max", int(max)), xlog.Int("min", int(min)))
		time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.syncBroker)
	}
}
func (b *broker) Shutdown(ctx context.Context) (err error) {
	for _, l := range b.listeners {
		if err = l.Shutdown(ctx); err != nil {
			b.logger.Error("listener shutdown", xlog.Err(err))
		}
	}
	if err = b.muticast.Shutdown(); err != nil {
		b.logger.Error("muticast shutdown", xlog.Err(err))
	}
	if err = b.httpServer.Shutdown(ctx); err != nil {
		b.logger.Error("http server shutdown", xlog.Err(err))
	}
	if err = b.peerServer.Shutdown(ctx); err != nil {
		b.logger.Error("peer server shutdown", xlog.Err(err))
	}
	return err
}
func (b *broker) freeMemory() {
	debug.FreeOSMemory()
	b.peers.Range(func(key string, p *peer) bool {
		p.freeMemory(context.Background())
		return true
	})
}
func (b *broker) IsOnline(ctx context.Context, uid string) (online bool) {
	if online = b.isOnline(uid); online {
		return true
	}
	b.peers.Range(func(id string, peer *peer) bool {
		online, err := peer.isOnline(ctx, uid)
		if err != nil {
			b.logger.Error("check online from peer failed", xlog.Err(err))
			return true
		}
		if online {
			online = true
			return false
		}
		return true
	})
	return online
}
func (b *broker) KickConn(ctx context.Context, cid string) {
	b.kickConn(cid)
	b.peers.Range(func(id string, peer *peer) bool {
		if err := peer.kickConn(ctx, cid); err != nil {
			b.logger.Error("kick conn from peer failed", xlog.Err(err))
		}
		return true
	})
}
func (b *broker) KickUser(ctx context.Context, uid string) {
	b.kickUser(uid)
	b.peers.Range(func(id string, peer *peer) bool {
		if err := peer.kickUser(ctx, uid); err != nil {
			b.logger.Error("kick user from peer failed", xlog.Err(err))
		}
		return true
	})
}
func (b *broker) SendToAll(ctx context.Context, m *packet.Message) (total, success uint64, err error) {
	total, success = b.sendToAll(m)
	b.peers.Range(func(id string, peer *peer) bool {
		if t, s, err := peer.sendMessage(ctx, m, "", 0); err == nil {
			total += t
			success += s
		} else {
			b.logger.Error("send to all from peer failed", xlog.Err(err))
		}
		return true
	})
	return total, success, nil
}
func (b *broker) SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success uint64, err error) {
	if uid == "" {
		return 0, 0, xerr.InvalidUserID
	}
	total, success = b.sendToUser(uid, m)
	b.peers.Range(func(id string, peer *peer) bool {
		t, s, err := peer.sendMessage(ctx, m, uid, 1)
		if err != nil {
			b.logger.Error("send to user from peer failed", xlog.Err(err))
			return true
		}
		total += t
		success += s
		return true
	})
	return total, success, nil
}
func (b *broker) SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success uint64, err error) {
	if channel == "" {
		return 0, 0, xerr.InvalidChannel
	}
	total, success = b.sendToChannel(channel, m)
	b.peers.Range(func(id string, peer *peer) bool {
		if t, s, err := peer.sendMessage(ctx, m, channel, 2); err == nil {
			total += t
			success += s
		} else {
			b.logger.Error("send to channel from peer failed", xlog.Err(err))
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
		} else {
			b.logger.Error("join channel from peer failed", xlog.Err(err))
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
		} else {
			b.logger.Error("leave channel from peer failed", xlog.Err(err))
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
		return nil, xerr.RequestHandlerNotFound
	}
	return handler(p, id)
}
func (b *broker) handlePeer(method string, handler server.RequestHandler) {
	b.peerHandlers.Set(method, handler)
}
func (b *broker) onPeerRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	handler, ok := b.peerHandlers.Get(p.Method)
	if !ok {
		return nil, xerr.RequestHandlerNotFound
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
func (b *broker) sendToAll(m *packet.Message) (total, success uint64) {
	for _, l := range b.listeners {
		t, s, err := l.Brodcast(m)
		total += t
		success += s
		if err != nil {
			b.logger.Error("send message to all failed", xlog.Err(err))
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
			b.logger.Error("send message to user failed", xlog.Err(err))
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
			b.logger.Error("send message to channel failed", xlog.Err(err))
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
