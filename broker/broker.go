package broker

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"sutext.github.io/cable/broker/protos"
	"sutext.github.io/cable/internal/discovery"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Broker interface {
	Start() error
	Shutdown(ctx context.Context) error
	Inspects(ctx context.Context) ([]*protos.Status, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickUser(ctx context.Context, uid string)
	SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error)
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error)
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) error
	LeaveChannel(ctx context.Context, uid string, channels ...string) error
	HandleRequest(method string, handler server.RequestHandler)
}

type broker struct {
	id             uint64
	ready          atomic.Bool
	peers          safe.RMap[uint64, *peerClient]
	logger         *xlog.Logger
	handler        Handler
	peerPort       string
	discovery      discovery.Discovery
	listeners      map[string]server.Server
	peerServer     *peerServer
	httpServer     *http.Server
	clusterSize    int32
	userClients    safe.RMap[uint64, *safe.KeyMap[string]] //pid map[uid]map[cid]net
	channelClients safe.RMap[uint64, *safe.KeyMap[string]] //pid map[channel]map[cid]net
	clientChannels safe.KeyMap[struct{}]                   //map[cid]map[channel]net
	requstHandlers safe.RMap[string, server.RequestHandler]
	raftNode       raft.Node
	raftStop       chan struct{}
	raftStorage    *raft.MemoryStorage
	raftLeader     atomic.Uint64
	isLeader       atomic.Bool
	confState      *raftpb.ConfState
	appliedIndex   uint64
	snapshotIndex  uint64
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{
		id:          options.brokerID,
		raftStop:    make(chan struct{}),
		clusterSize: options.clusterSize,
	}
	b.logger = xlog.With("BROKER", b.id)
	b.handler = options.handler
	b.discovery = discovery.New(b.id, options.peerPort)
	b.discovery.OnRequest(func(id uint64, addr string) {
		b.addPeer(id, addr)
	})
	b.listeners = make(map[string]server.Server, len(options.listeners))
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
	mux.HandleFunc("/removeNode", b.handleRemoveNode)
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
			b.addRaftNode(context.Background(), id)
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
		if err := b.discovery.Serve(); err != nil {
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
	time.AfterFunc(time.Second, b.autoDiscovery)
	return nil
}

func (b *broker) autoDiscovery() {
	m, err := b.discovery.Request()
	if err != nil {
		b.logger.Error("failed to sync broker", xlog.Err(err))
	}
	if len(m) == 0 {
		b.logger.Warn("Auto discovery got empty endpoints")
		go b.autoDiscovery()
		return
	}
	for id, addr := range m {
		b.addPeer(id, addr)
	}
	if b.peers.Len() < b.clusterSize-1 {
		b.logger.Warn("Auto discovery got less than cluster size", xlog.I32("clusterSize", b.clusterSize), xlog.I32("peers", b.peers.Len()))
		go b.autoDiscovery()
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
	if err = b.discovery.Shutdown(); err != nil {
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
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		if id == b.id {
			if cids, ok := value.Get(uid); ok {
				if b.isActive(cids.Load()) {
					online = true
					return false
				}
			}
		} else {
			if peer, ok := b.peers.Get(id); ok {
				if cids, ok := value.Get(uid); ok {
					if ok, err := peer.isOnline(ctx, cids.Load()); err == nil && ok {
						online = true
						return false
					}
				}
			}
		}
		return true
	})
	return online
}

func (b *broker) KickUser(ctx context.Context, uid string) {
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		if id == b.id {
			if cids, ok := value.Get(uid); ok {
				b.kickConn(cids.Load())
			}
		} else {
			if peer, ok := b.peers.Get(id); ok {
				if cids, ok := value.Get(uid); ok {
					if err := peer.kickConn(ctx, cids.Load()); err != nil {
						b.logger.Error("kick user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
					}
				}
			}
		}
		return true
	})
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
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		if id == b.id {
			if cids, ok := value.Get(uid); ok {
				t, s := b.sendToTargets(ctx, m, cids.Load())
				total += t
				success += s
			}
		} else {
			if peer, ok := b.peers.Get(id); ok {
				if cids, ok := value.Get(uid); ok {
					t, s, err := peer.sendToTargets(ctx, m, cids.Load())
					if err == nil {
						total += t
						success += s
					} else {
						b.logger.Error("send to user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
					}
				}
			}
		}
		return true
	})
	return total, success, nil
}
func (b *broker) SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error) {
	if channel == "" {
		return 0, 0, xerr.InvalidChannel
	}
	b.channelClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		if id == b.id {
			if cids, ok := value.Get(channel); ok {
				t, s := b.sendToTargets(ctx, m, cids.Load())
				total += t
				success += s
			}
		} else {
			if peer, ok := b.peers.Get(id); ok {
				if cids, ok := value.Get(channel); ok {
					t, s, err := peer.sendToTargets(ctx, m, cids.Load())
					if err == nil {
						total += t
						success += s
					} else {
						b.logger.Error("send to user from peer failed", xlog.Peer(id), xlog.Channel(channel), xlog.Err(err))
					}
				}
			}
		}
		return true
	})
	return total, success, nil
}

func (b *broker) JoinChannel(ctx context.Context, uid string, channels ...string) error {
	op := &joinChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.submitRaftOp(ctx, op)
}

func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) error {
	op := &leaveChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.submitRaftOp(ctx, op)
}
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.requstHandlers.Set(method, handler)
}
func (b *broker) onUserClosed(id *packet.Identity) {
	op := &userClosedOp{
		uid: id.UserID,
		cid: id.ClientID,
	}
	if err := b.submitRaftOp(context.Background(), op); err != nil {
		b.logger.Error("Failed to submit user disconnect op", xlog.Err(err))
	}
	b.handler.OnClosed(id)
}
func (b *broker) onUserConnect(p *packet.Connect, net string) packet.ConnectCode {
	code := b.handler.OnConnect(p)
	if code != packet.ConnectAccepted {
		return code
	}
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		if cnet, ok := value.GetKey(p.Identity.UserID, p.Identity.ClientID); ok {
			if id != b.id || net != cnet {
				b.logger.Info("duplicate connection detected, kick old connection",
					xlog.Uid(p.Identity.UserID),
					xlog.Cid(p.Identity.ClientID),
					xlog.U64("old_peer_id", id),
					xlog.Str("old_net", cnet),
					xlog.U64("new_peer_id", b.id),
					xlog.Str("new_net", net),
				)
				if id == b.id {
					if l, ok := b.listeners[cnet]; ok {
						l.KickConn(p.Identity.ClientID)
					}
				} else {
					if peer, ok := b.peers.Get(id); ok {
						cids := map[string]string{p.Identity.ClientID: net}
						if err := peer.kickConn(context.Background(), cids); err != nil {
							b.logger.Error("kick old connection from peer failed", xlog.Peer(id), xlog.Err(err))
						}
					}
				}
			}
		}
		return true
	})
	op := &userOpenedOp{
		uid:      p.Identity.UserID,
		cid:      p.Identity.ClientID,
		net:      string(net),
		brokerID: b.id,
	}
	if err := b.submitRaftOp(context.Background(), op); err != nil {
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

func (b *broker) isActive(targets map[string]string) bool {
	for cid, net := range targets {
		if l, ok := b.listeners[net]; ok {
			if l.IsActive(cid) {
				return true
			}
		}
	}

	return false
}
func (b *broker) kickConn(targets map[string]string) {
	for cid, net := range targets {
		if l, ok := b.listeners[net]; ok {
			l.KickConn(cid)
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
func (b *broker) sendToTargets(ctx context.Context, m *packet.Message, targets map[string]string) (total, success int32) {
	for cid, net := range targets {
		total++
		if l, ok := b.listeners[net]; ok {
			if err := l.SendMessage(ctx, cid, m); err != nil {
				b.logger.Error("send message to cid failed", xlog.Cid(cid), xlog.Err(err))
			} else {
				success++
			}
		}
	}
	return total, success
}
