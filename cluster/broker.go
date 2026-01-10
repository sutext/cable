package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"sutext.github.io/cable/cluster/pb"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

type Broker interface {
	Start() error
	Cluster() Cluster
	Shutdown(ctx context.Context) error
	Inspects(ctx context.Context) ([]*pb.Status, error)
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
	logger         *xlog.Logger
	cluster        *cluster
	handler        Handler
	peerPort       string
	listeners      map[string]server.Server
	peerServer     *peerServer
	httpServer     *http.Server
	userClients    safe.RMap[uint64, *safe.KeyMap[string]]   //bid map[uid]map[cid]net
	channelClients safe.RMap[uint64, *safe.KeyMap[string]]   //bid map[channel]map[cid]net
	clientChannels safe.RMap[uint64, *safe.KeyMap[struct{}]] //bid map[cid]map[channel]null
	requstHandlers safe.RMap[string, server.RequestHandler]
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{
		id: options.brokerID,
	}
	b.logger = xlog.With("BROKER", b.id)
	b.handler = options.handler
	b.peerPort = options.peerPort
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
			server.WithQUICConfig(options.quicConfig),
		)
	}
	b.cluster = newCluster(b, options.initSize)
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

func (b *broker) Start() error {
	for _, l := range b.listeners {
		go func() {
			if err := l.Serve(); err != nil {
				panic(err)
			}
		}()
	}
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
	b.cluster.Start()
	return nil
}

func (b *broker) Cluster() Cluster {
	return b.cluster
}

func (b *broker) Shutdown(ctx context.Context) (err error) {
	for _, l := range b.listeners {
		if err = l.Shutdown(ctx); err != nil {
			b.logger.Error("listener shutdown", xlog.Err(err))
		}
	}
	if err = b.httpServer.Shutdown(ctx); err != nil {
		b.logger.Error("http server shutdown", xlog.Err(err))
	}
	if err = b.peerServer.Shutdown(ctx); err != nil {
		b.logger.Error("peer server shutdown", xlog.Err(err))
	}
	b.cluster.Stop()
	return err
}
func (b *broker) ExpelAllConns() {
	for _, l := range b.listeners {
		l.ExpelAllConns()
	}
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
			if peer, ok := b.cluster.GetPeer(id); ok {
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
			if peer, ok := b.cluster.GetPeer(id); ok {
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
	b.cluster.RangePeers(func(id uint64, peer *peerClient) {
		wg.Go(func() {
			if t, s, err := peer.sendToAll(ctx, m); err == nil {
				total += t
				success += s
			} else {
				b.logger.Error("send to all from peer failed", xlog.Peer(id), xlog.Err(err))
			}
		})
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
			if peer, ok := b.cluster.GetPeer(id); ok {
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
			if peer, ok := b.cluster.GetPeer(id); ok {
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
	return b.cluster.SubmitOperation(ctx, op)
}

func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) error {
	op := &leaveChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.cluster.SubmitOperation(ctx, op)
}
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.requstHandlers.Set(method, handler)
}
func (b *broker) onUserClosed(id *packet.Identity) {
	op := &userClosedOp{
		uid: id.UserID,
		cid: id.ClientID,
	}
	if err := b.cluster.SubmitOperation(context.Background(), op); err != nil {
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
					if peer, ok := b.cluster.GetPeer(id); ok {
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
	chs, err := b.handler.GetChannels(p.Identity.UserID)
	if err != nil {
		b.logger.Error("get channels failed", xlog.Err(err))
		chs = []string{}
	}
	op := &userOpenedOp{
		uid:      p.Identity.UserID,
		cid:      p.Identity.ClientID,
		net:      string(net),
		nodeID:   b.id,
		channels: chs,
	}
	if err := b.cluster.SubmitOperation(context.Background(), op); err != nil {
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
func (b *broker) userOpenedOp(op *userOpenedOp) {
	userClients, _ := b.userClients.GetOrSet(op.nodeID, &safe.KeyMap[string]{})
	userClients.SetKey(op.uid, op.cid, op.net)
	channelClients, _ := b.channelClients.GetOrSet(op.nodeID, &safe.KeyMap[string]{})
	clientChannels, _ := b.clientChannels.GetOrSet(op.nodeID, &safe.KeyMap[struct{}]{})
	for _, ch := range op.channels {
		clientChannels.SetKey(op.cid, ch, struct{}{})
		channelClients.SetKey(ch, op.cid, op.net)
	}
}

func (b *broker) userClosedOp(op *userClosedOp) {
	if userClients, ok := b.userClients.Get(op.nodeID); ok {
		userClients.DeleteKey(op.uid, op.cid)
	}
	if clientChannels, ok := b.clientChannels.Get(op.nodeID); ok {
		if channelClients, ok := b.channelClients.Get(op.nodeID); ok {
			clientChannels.RangeKey(op.cid, func(channel string, s2 struct{}) bool {
				channelClients.DeleteKey(channel, op.cid)
				return true
			})
		}
		clientChannels.DeleteKey(op.cid, op.uid)
	}
}

func (b *broker) joinChannelOp(op *joinChannelOp) {
	b.userClients.Range(func(id uint64, userClients *safe.KeyMap[string]) bool {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		clientChannels, _ := b.clientChannels.GetOrSet(id, &safe.KeyMap[struct{}]{})
		userClients.RangeKey(op.uid, func(cid string, net string) bool {
			for _, channel := range op.channels {
				channelClients.SetKey(channel, cid, net)
				clientChannels.SetKey(cid, channel, struct{}{})
			}
			return true
		})
		return true
	})
}
func (b *broker) leaveChannelOp(op *leaveChannelOp) {
	b.userClients.Range(func(id uint64, userClients *safe.KeyMap[string]) bool {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		clientChannels, _ := b.clientChannels.GetOrSet(id, &safe.KeyMap[struct{}]{})
		userClients.RangeKey(op.uid, func(cid string, net string) bool {
			for _, ch := range op.channels {
				channelClients.DeleteKey(ch, cid)
				clientChannels.DeleteKey(cid, ch)
			}
			return true
		})
		return true
	})
}
func (b *broker) makeSnapshot() ([]byte, error) {
	userClients := make(map[uint64]map[string]map[string]string)
	b.userClients.Range(func(id uint64, v *safe.KeyMap[string]) bool {
		ucmap, ok := userClients[id]
		if !ok {
			ucmap = make(map[string]map[string]string)
			userClients[id] = ucmap
		}
		v.Range(func(uid string, cids *safe.Map[string, string]) bool {
			ucmap[uid] = cids.Load()
			return true
		})
		return true
	})
	channelClients := make(map[uint64]map[string]map[string]string)
	b.channelClients.Range(func(id uint64, v *safe.KeyMap[string]) bool {
		ccmap, ok := channelClients[id]
		if !ok {
			ccmap = make(map[string]map[string]string)
			channelClients[id] = ccmap
		}
		v.Range(func(cid string, uids *safe.Map[string, string]) bool {
			ccmap[cid] = uids.Load()
			return true
		})
		return true
	})
	clientChannels := make(map[uint64]map[string]map[string]struct{})
	b.clientChannels.Range(func(id uint64, v *safe.KeyMap[struct{}]) bool {
		ccmap, ok := clientChannels[id]
		if !ok {
			ccmap = make(map[string]map[string]struct{})
			clientChannels[id] = ccmap
		}
		v.Range(func(cid string, channels *safe.Map[string, struct{}]) bool {
			ccmap[cid] = channels.Load()
			return true
		})
		return true
	})
	snapshot := snapshot{
		UserClients:    userClients,
		ChannelClients: channelClients,
		ClientChannels: clientChannels,
	}
	return json.Marshal(snapshot)
}
func (b *broker) recoverFromSnapshot(s snapshot) {
	b.userClients = safe.RMap[uint64, *safe.KeyMap[string]]{}
	b.channelClients = safe.RMap[uint64, *safe.KeyMap[string]]{}
	b.clientChannels = safe.RMap[uint64, *safe.KeyMap[struct{}]]{}
	for id, ucmap := range s.UserClients {
		userClients, _ := b.userClients.GetOrSet(id, &safe.KeyMap[string]{})
		for uid, cids := range ucmap {
			userClients.Set(uid, safe.NewMap(cids))
		}
	}
	for id, ccmap := range s.ChannelClients {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		for cid, uids := range ccmap {
			channelClients.Set(cid, safe.NewMap(uids))
		}
	}
	for id, ccmap := range s.ClientChannels {
		clientChannels, _ := b.clientChannels.GetOrSet(id, &safe.KeyMap[struct{}]{})
		for cid, channels := range ccmap {
			clientChannels.Set(cid, safe.NewMap(channels))
		}
	}
}
