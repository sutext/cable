package broker

import (
	"context"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/raft/v3"
	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/internal/muticast"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Broker interface {
	Start() error
	Shutdown(ctx context.Context) error
	Inspects(ctx context.Context) ([]*protos.Inspects, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickUser(ctx context.Context, uid string)
	SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error)
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error)
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) error
	LeaveChannel(ctx context.Context, uid string, channels ...string) error
	HandleRequest(method string, handler server.RequestHandler)
}

type idAndNet struct {
	id  uint64
	net string
}
type broker struct {
	id             uint64
	ready          atomic.Bool
	peers          safe.Map[uint64, *peerClient]
	logger         *xlog.Logger
	handler        Handler
	muticast       muticast.Muticast
	listeners      map[server.Transport]server.Server
	peerPort       string
	peerServer     *peerServer
	httpServer     *http.Server
	clusterSize    int32
	userClients    safe.KeyMap[idAndNet] //map[uid]map[cid]net
	channelClients safe.KeyMap[idAndNet] //map[channel]map[cid]net
	clientChannels safe.KeyMap[struct{}] //map[cid]map[channel]net
	requstHandlers safe.Map[string, server.RequestHandler]
	//for raft
	node       raft.Node
	shutdownCh chan struct{}
	storage    *raft.MemoryStorage
	leaderID   atomic.Uint64 // 当前leader节点ID
	isLeader   atomic.Bool   // 当前节点是否是leader
	lastLeader time.Time     // 最后一次看到leader的时间
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{
		id:          options.brokerID,
		shutdownCh:  make(chan struct{}),
		clusterSize: options.clusterSize,
	}
	b.logger = xlog.With("BROKER", b.id)
	b.handler = options.handler
	b.muticast = muticast.New(b.id, options.peerPort)
	b.muticast.OnRequest(func(id uint64, addr string) {
		b.addPeer(id, addr)
	})
	b.listeners = make(map[server.Transport]server.Server, len(options.listeners))
	for net, addr := range options.listeners {
		b.listeners[net] = server.New(addr,
			server.WithClose(b.onUserClosed),
			server.WithLogger(b.logger),
			server.WithTransport(net),
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
	// mux.HandleFunc("/join", b.handleJoin)
	mux.HandleFunc("/inspect", b.handleInspect)
	mux.HandleFunc("/kickout", b.handleKickout)
	mux.HandleFunc("/message", b.handleMessage)
	mux.HandleFunc("/brodcast", b.handleBrodcast)
	mux.HandleFunc("/health", b.handleHealth)
	b.httpServer = &http.Server{Addr: options.httpPort, Handler: mux}
	return b
}

func (b *broker) addPeer(id uint64, addr string) {
	if id == b.id {
		return
	}
	if p, ok := b.peers.Get(id); ok {
		p.updateAddr(addr)
		return
	}
	peer := newPeerClient(id, addr, b.logger)
	b.peers.Set(id, peer)
	peer.connect()
	if b.peers.Len() > b.clusterSize-1 {
		if b.isLeader.Load() {
			b.addRaftNode(id)
		}
	}
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
	time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.autoDiscovery)
	return nil
}

func (b *broker) autoDiscovery() {
	m, err := b.muticast.Request()
	if err != nil {
		b.logger.Error("failed to sync broker", xlog.Err(err))
	}
	if len(m) == 0 {
		b.logger.Warn("Auto discovery got empty endpoints")
		time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.autoDiscovery)
	}
	for id, addr := range m {
		if id == b.id {
			continue
		}
		b.addPeer(id, addr)
	}

	if b.peers.Len() < b.clusterSize-1 {
		b.logger.Warn("Auto discovery got less than cluster size", xlog.Int("clusterSize", int(b.clusterSize)), xlog.I32("peers", b.peers.Len()))
		time.AfterFunc(time.Second*time.Duration(1+rand.IntN(4)), b.autoDiscovery)
	} else if b.peers.Len() == b.clusterSize-1 {
		b.startRaft(false)
	} else {
		b.startRaft(true)
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
func (b *broker) groupedUsers(uid string) map[uint64][]*protos.Target {
	grouped := make(map[uint64][]*protos.Target)
	b.userClients.RangeKey(uid, func(cid string, ian idAndNet) bool {
		ts := grouped[ian.id]
		ts = append(ts, &protos.Target{Net: ian.net, Cid: cid})
		grouped[ian.id] = ts
		return true
	})
	return grouped
}
func (b *broker) groupedChannels(channel string) map[uint64][]*protos.Target {
	grouped := make(map[uint64][]*protos.Target)
	b.channelClients.RangeKey(channel, func(cid string, ian idAndNet) bool {
		ts := grouped[ian.id]
		ts = append(ts, &protos.Target{Net: ian.net, Cid: cid})
		grouped[ian.id] = ts
		return true
	})
	return grouped
}
func (b *broker) IsOnline(ctx context.Context, uid string) (online bool) {
	m := b.groupedUsers(uid)
	for id, targets := range m {
		if id == b.id {
			if b.isActive(targets) {
				return true
			}
		} else {
			if peer, ok := b.peers.Get(id); ok {
				if ok, err := peer.isOnline(ctx, targets); err == nil && ok {
					return true
				}
			}
		}
	}
	return false
}

func (b *broker) KickUser(ctx context.Context, uid string) {
	m := b.groupedUsers(uid)
	for id, targets := range m {
		if id == b.id {
			b.kickConn(targets)
		} else {
			if peer, ok := b.peers.Get(id); ok {
				peer.kickConn(ctx, targets)
			}
		}
	}
}
func (b *broker) SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error) {
	total, success = b.sendToAll(ctx, m)
	wg := sync.WaitGroup{}
	b.peers.Range(func(id uint64, peer *peerClient) bool {
		wg.Go(func() {
			if t, s, err := peer.sendToAll(ctx, m); err == nil {
				total += t
				success += s
			} else {
				b.logger.Error("send to all from peer failed", xlog.Peer(id), xlog.Err(err))
			}
		})
		return true
	})
	wg.Wait()
	return total, success, nil
}
func (b *broker) SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error) {
	if uid == "" {
		return 0, 0, xerr.InvalidUserID
	}
	chs := b.groupedUsers(uid)
	for id, targets := range chs {
		if id == b.id {
			t, s := b.sendToTargets(ctx, m, targets)
			total += t
			success += s
		} else {
			if peer, ok := b.peers.Get(id); ok {
				t, s, err := peer.sendToTargets(ctx, m, targets)
				if err == nil {
					total += t
					success += s
				} else {
					b.logger.Error("send to user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
				}
			}
		}
	}
	return total, success, nil
}
func (b *broker) SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error) {
	if channel == "" {
		return 0, 0, xerr.InvalidChannel
	}
	chs := b.groupedChannels(channel)
	for id, targets := range chs {
		if id == b.id {
			t, s := b.sendToTargets(ctx, m, targets)
			total += t
			success += s
		} else {
			if peer, ok := b.peers.Get(id); ok {
				t, s, err := peer.sendToTargets(ctx, m, targets)
				if err == nil {
					total += t
					success += s
				} else {
					b.logger.Error("send to user from peer failed", xlog.Peer(id), xlog.Channel(channel), xlog.Err(err))
				}
			}
		}
	}
	return total, success, nil
}

func (b *broker) JoinChannel(ctx context.Context, uid string, channels ...string) error {
	op := &joinChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.SubmitOperation(op)
}

func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) error {
	op := &leaveChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.SubmitOperation(op)
}
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.requstHandlers.Set(method, handler)
}
func (b *broker) onUserClosed(id *packet.Identity) {
	op := &userClosedOp{
		uid: id.UserID,
		cid: id.ClientID,
	}

	if err := b.SubmitOperation(op); err != nil {
		b.logger.Error("Failed to submit user disconnect op", xlog.Err(err))
	}
	b.handler.OnClosed(id)
}
func (b *broker) onUserConnect(p *packet.Connect, net server.Transport) packet.ConnectCode {
	code := b.handler.OnConnect(p)
	if code != packet.ConnectAccepted {
		return code
	}
	if id, ok := b.userClients.GetKey(p.Identity.UserID, p.Identity.ClientID); ok {
		if id.id != b.id || id.net != string(net) {
			b.logger.Info("duplicate connection detected, kick old connection",
				xlog.Uid(p.Identity.UserID),
				xlog.Cid(p.Identity.ClientID),
				xlog.Peer(id.id),
				xlog.Str("old_net", id.net),
				xlog.Peer(b.id),
				xlog.Str("new_net", string(net)),
			)
			if id.id == b.id {
				if l, ok := b.listeners[server.Transport(id.net)]; ok {
					l.KickConn(p.Identity.ClientID)
				}
			} else {
				if peer, ok := b.peers.Get(id.id); ok {
					t := protos.Target{Net: id.net, Cid: p.Identity.ClientID}
					if err := peer.kickConn(context.Background(), []*protos.Target{&t}); err != nil {
						b.logger.Error("kick old connection from peer failed", xlog.Peer(id.id), xlog.Err(err))
					}
				}
			}
		}
	}

	op := &userOpenedOp{
		uid:      p.Identity.UserID,
		cid:      p.Identity.ClientID,
		net:      string(net),
		brokerID: b.id,
	}
	if err := b.SubmitOperation(op); err != nil {
		b.logger.Error("Failed to submit user connect op", xlog.Err(err))
		return packet.ConnectRejected
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

func (b *broker) isActive(targets []*protos.Target) bool {
	for _, target := range targets {
		if l, ok := b.listeners[server.Transport(target.Net)]; ok {
			if l.IsActive(target.Cid) {
				return true
			}
		}
	}

	return false
}
func (b *broker) kickConn(targets []*protos.Target) {
	for _, target := range targets {
		if l, ok := b.listeners[server.Transport(target.Net)]; ok {
			l.KickConn(target.Cid)
		}
	}
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
func (b *broker) sendToTargets(ctx context.Context, m *packet.Message, targets []*protos.Target) (total, success int32) {
	for _, t := range targets {
		total++
		if l, ok := b.listeners[server.Transport(t.Net)]; ok {
			if err := l.SendMessage(ctx, t.Cid, m); err != nil {
				b.logger.Error("send message to cid failed", xlog.Cid(t.Cid), xlog.Err(err))
			} else {
				success++
			}
		}
	}
	return total, success
}
