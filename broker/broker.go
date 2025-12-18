package broker

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"sutext.github.io/cable/broker/protos"
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
	Inspects() ([]*protos.Inspects, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickConn(ctx context.Context, cid string)
	KickUser(ctx context.Context, uid string)
	SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error)
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error)
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) (count int32, err error)
	LeaveChannel(ctx context.Context, uid string, channels ...string) (count int32, err error)
	HandleRequest(method string, handler server.RequestHandler)
}
type broker struct {
	id             string
	peers          safe.Map[string, *peerClient]
	logger         *xlog.Logger
	handler        Handler
	muticast       muticast.Muticast
	listeners      map[server.Network]server.Server
	peerPort       string
	peerServer     *peerServer
	httpServer     *http.Server
	userClients    safe.KeyMap[server.Network] //map[uid]map[cid]net
	channelClients safe.KeyMap[server.Network] //map[channel]map[cid]net
	clientChannels safe.KeyMap[server.Network] //map[cid]map[channel]net
	requstHandlers safe.Map[string, server.RequestHandler]
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.logger = xlog.With("BROKER", b.id)
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
			server.WithLogger(b.logger),
			server.WithNetwork(net),
			server.WithMessage(b.onUserMessage),
			server.WithRequest(b.onUserRequest),
			server.WithConnect(func(p *packet.Connect) packet.ConnectCode {
				return b.onUserConnect(p, net)
			}),
		)
	}
	b.peerPort = options.peerPort
	b.peerServer = newPeerServer(b, b.peerPort)
	mux := http.NewServeMux()
	mux.HandleFunc("/join", b.handleJoin)
	mux.HandleFunc("/inspect", b.handleInspect)
	mux.HandleFunc("/kickout", b.handleKickout)
	mux.HandleFunc("/message", b.handleMessage)
	mux.HandleFunc("/brodcast", b.handleBrodcast)
	mux.HandleFunc("/health", b.handleHealth)
	b.httpServer = &http.Server{Addr: options.httpPort, Handler: mux}
	return b
}
func (b *broker) clusterSize() int32 {
	return b.peers.Len() + 1
}
func (b *broker) delPeer(id string) {
	b.logger.Debug("del peer", xlog.Peer(id))
	if _, ok := b.peers.Get(id); ok {
		b.peers.Delete(id)
	}
}
func (b *broker) addPeer(id, ip string) {
	if id == b.id {
		return
	}
	if p, ok := b.peers.Get(id); ok {
		p.UpdateIP(ip)
		return
	}
	peer := newPeerClient(id, ip, b)
	if peer == nil {
		return
	}
	b.logger.Debug("add peer", xlog.Peer(id))
	b.peers.Set(id, peer)
	peer.connect()
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
	min := b.clusterSize()
	max := b.clusterSize()
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
	b.peers.Range(func(id string, peer *peerClient) bool {
		if _, ok := m[id+":"+peer.ip]; !ok {
			b.delPeer(id)
		}
		return true
	})
	if max != min {
		b.logger.Warn("broker count mismatch", xlog.I32("max", max), xlog.I32("min", min))
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
func (b *broker) IsOnline(ctx context.Context, uid string) (online bool) {
	if online = b.isOnline(uid); online {
		return true
	}
	b.peers.Range(func(id string, peer *peerClient) bool {
		online, err := peer.isOnline(ctx, uid)
		if err != nil {
			b.logger.Error("check online from peer failed", xlog.Str("peer", id), xlog.Uid(uid), xlog.Err(err))
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
	b.peers.Range(func(id string, peer *peerClient) bool {
		if err := peer.kickConn(ctx, cid); err != nil {
			b.logger.Error("kick conn from peer failed", xlog.Peer(id), xlog.Cid(cid), xlog.Err(err))
		}
		return true
	})
}
func (b *broker) KickUser(ctx context.Context, uid string) {
	b.kickUser(uid)
	b.peers.Range(func(id string, peer *peerClient) bool {
		if err := peer.kickUser(ctx, uid); err != nil {
			b.logger.Error("kick user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
		}
		return true
	})
}
func (b *broker) SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error) {
	total, success = b.sendToAll(ctx, m)
	b.peers.Range(func(id string, peer *peerClient) bool {
		if t, s, err := peer.sendMessage(ctx, m, "", 0); err == nil {
			total += t
			success += s
		} else {
			b.logger.Error("send to all from peer failed", xlog.Peer(id), xlog.Err(err))
		}
		return true
	})
	return total, success, nil
}
func (b *broker) SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error) {
	if uid == "" {
		return 0, 0, xerr.InvalidUserID
	}
	total, success = b.sendToUser(ctx, uid, m)
	b.peers.Range(func(id string, peer *peerClient) bool {
		t, s, err := peer.sendMessage(ctx, m, uid, 1)
		if err != nil {
			b.logger.Error("send to user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
			return true
		}
		total += t
		success += s
		return true
	})
	return total, success, nil
}
func (b *broker) SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error) {
	if channel == "" {
		return 0, 0, xerr.InvalidChannel
	}
	total, success = b.sendToChannel(ctx, channel, m)
	b.peers.Range(func(id string, peer *peerClient) bool {
		if t, s, err := peer.sendMessage(ctx, m, channel, 2); err == nil {
			total += t
			success += s
		} else {
			b.logger.Error("send to channel from peer failed", xlog.Peer(id), xlog.Channel(channel), xlog.Err(err))
		}
		return true
	})
	return total, success, nil
}
func (b *broker) JoinChannel(ctx context.Context, uid string, channels ...string) (count int32, err error) {
	count = b.joinChannel(uid, channels)
	b.peers.Range(func(id string, peer *peerClient) bool {
		if c, err := peer.joinChannel(ctx, uid, channels); err == nil {
			count += c
		} else {
			b.logger.Error("join channel from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
		}
		return true
	})
	return count, nil
}
func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) (count int32, err error) {
	count = b.leaveChannel(uid, channels)
	b.peers.Range(func(id string, peer *peerClient) bool {
		if c, err := peer.leaveChannel(ctx, uid, channels); err == nil {
			count += c
		} else {
			b.logger.Error("leave channel from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
		}
		return true
	})
	return count, nil
}
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.requstHandlers.Set(method, handler)
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
func (b *broker) onUserConnect(p *packet.Connect, net server.Network) packet.ConnectCode {
	code := b.handler.OnConnect(p)
	if code != packet.ConnectAccepted {
		return code
	}
	b.peers.Range(func(id string, peer *peerClient) bool {
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
	handler, ok := b.requstHandlers.Get(p.Method)
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
func (b *broker) sendToAll(ctx context.Context, m *packet.Message) (total, success int32) {
	for _, l := range b.listeners {
		t, s, err := l.Brodcast(ctx, m)
		total += t
		success += s
		if err != nil {
			b.logger.Error("send message to all failed", xlog.Err(err))
		}
	}
	return total, success
}
func (b *broker) sendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32) {
	b.userClients.RangeKey(uid, func(cid string, net server.Network) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			return true
		}
		if err := l.SendMessage(ctx, cid, m); err != nil {
			b.logger.Error("send message to user failed", xlog.Uid(uid), xlog.Err(err))
		} else {
			success++
		}
		return true
	})
	return total, success
}
func (b *broker) sendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32) {
	b.channelClients.RangeKey(channel, func(cid string, net server.Network) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			return true
		}
		if err := l.SendMessage(ctx, cid, m); err != nil {
			b.logger.Error("send message to channel failed", xlog.Channel(channel), xlog.Err(err))
		} else {
			success++
		}
		return true
	})
	return total, success
}

func (b *broker) joinChannel(uid string, channels []string) (count int32) {
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
func (b *broker) leaveChannel(uid string, channels []string) (count int32) {
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
