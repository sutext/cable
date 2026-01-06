package broker

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
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

type BrokerID string
type Broker interface {
	Start() error
	Shutdown(ctx context.Context) error
	Inspects(ctx context.Context) ([]*protos.Inspects, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickUser(ctx context.Context, uid string)
	SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error)
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error)
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error)
	// JoinChannel(ctx context.Context, uid string, channels ...string) (count int32, err error)
	// LeaveChannel(ctx context.Context, uid string, channels ...string) (count int32, err error)
	HandleRequest(method string, handler server.RequestHandler)
}

type idAndNet struct {
	id  string
	net string
}
type broker struct {
	id             string
	node           raft.Node
	peers          safe.Map[string, *peerClient]
	logger         *xlog.Logger
	handler        Handler
	muticast       muticast.Muticast
	listeners      map[server.Transport]server.Server
	peerPort       string
	peerServer     *peerServer
	httpServer     *http.Server
	userClients    safe.KeyMap[idAndNet] //map[uid]map[cid]net
	channelClients safe.KeyMap[idAndNet] //map[channel]map[cid]net
	clientChannels safe.KeyMap[struct{}] //map[cid]map[channel]net
	requstHandlers safe.Map[string, server.RequestHandler]
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.logger = xlog.With("BROKER", b.id)
	b.handler = options.handler
	b.muticast = muticast.New(b.id, options.peerPort)
	b.muticast.OnRequest(func(idaddr string) int32 {
		strs := strings.Split(idaddr, "@")
		b.addPeer(strs[0], strs[1])
		return b.clusterSize()
	})
	for _, idaddr := range options.peers {
		strs := strings.Split(idaddr, "@")
		if len(strs) != 2 {
			panic("peers format error want id@ip:port")
		}
		b.addPeer(strs[0], strs[1])
	}
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
func (b *broker) startNode() {
	storage := raft.NewMemoryStorage()
	c := &raft.Config{
		ID:              0x01,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   4096,
		MaxInflightMsgs: 256,
	}
	// Set peer list to all nodes in the cluster.
	// Note that other nodes in the cluster, apart from this node, need to be started separately.
	b.node = raft.StartNode(c, nil)
}
func (b *broker) clusterSize() int32 {
	return b.peers.Len() + 1
}
func (b *broker) delPeer(id string) {
	if b.peers.Delete(id) {
		b.logger.Info("peer deleted", xlog.Peer(id))
	}
}
func (b *broker) addPeer(id, addr string) {
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
	for idaddr, count := range m {
		if count > max {
			max = count
		}
		if count < min {
			min = count
		}
		strs := strings.Split(idaddr, "@")
		b.addPeer(strs[0], strs[1])
	}
	b.peers.Range(func(id string, peer *peerClient) bool {
		if _, ok := m[id+"@"+peer.addr]; !ok {
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
func (b *broker) groupedUsers(uid string) map[string][]*protos.Target {
	grouped := make(map[string][]*protos.Target)
	b.userClients.RangeKey(uid, func(cid string, ian idAndNet) bool {
		ts := grouped[ian.net]
		ts = append(ts, &protos.Target{Net: ian.net, Cid: cid})
		grouped[ian.id] = ts
		return true
	})
	return grouped
}
func (b *broker) groupedChannels(channel string) map[string][]*protos.Target {
	grouped := make(map[string][]*protos.Target)
	b.channelClients.RangeKey(channel, func(cid string, ian idAndNet) bool {
		ts := grouped[ian.net]
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
	// if online = b.isOnline(uid); online {
	// 	return true
	// }
	// b.peers.Range(func(id string, peer *peerClient) bool {
	// 	ok, err := peer.isOnline(ctx, uid)
	// 	if err != nil {
	// 		b.logger.Error("check online from peer failed", xlog.Str("peer", id), xlog.Uid(uid), xlog.Err(err))
	// 		return true
	// 	}
	// 	if ok {
	// 		online = true
	// 		return false
	// 	}
	// 	return true
	// })
	// return online
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
	// b.kickUser(uid)
	// wg := sync.WaitGroup{}
	// b.peers.Range(func(id string, peer *peerClient) bool {
	// 	wg.Go(func() {
	// 		if err := peer.kickUser(ctx, uid); err != nil {
	// 			b.logger.Error("kick user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
	// 		}
	// 	})
	// 	return true
	// })
	// wg.Wait()
}
func (b *broker) SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error) {
	total, success = b.sendToAll(ctx, m)
	wg := sync.WaitGroup{}
	b.peers.Range(func(id string, peer *peerClient) bool {
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
	// total, success = b.sendToUser(ctx, uid, m)
	// wg := sync.WaitGroup{}
	// b.peers.Range(func(id string, peer *peerClient) bool {
	// 	wg.Go(func() {
	// 		if t, s, err := peer.sendMessage(ctx, m, uid, 1); err == nil {
	// 			total += t
	// 			success += s
	// 		} else {
	// 			b.logger.Error("send to user from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
	// 		}
	// 	})
	// 	return true
	// })
	// wg.Wait()
	// return total, success, nil
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

	// total, success = b.sendToChannel(ctx, channel, m)
	// wg := sync.WaitGroup{}
	// b.peers.Range(func(id string, peer *peerClient) bool {
	// 	wg.Go(func() {
	// 		if t, s, err := peer.sendMessage(ctx, m, channel, 2); err == nil {
	// 			total += t
	// 			success += s
	// 		} else {
	// 			b.logger.Error("send to channel from peer failed", xlog.Peer(id), xlog.Channel(channel), xlog.Err(err))
	// 		}
	// 	})
	// 	return true
	// })
	// wg.Wait()
	// return total, success, nil
}

//	func (b *broker) JoinChannel(ctx context.Context, uid string, channels ...string) (count int32, err error) {
//		count = b.joinChannel(uid, channels)
//		wg := sync.WaitGroup{}
//		b.peers.Range(func(id string, peer *peerClient) bool {
//			wg.Go(func() {
//				if c, err := peer.joinChannel(ctx, uid, channels); err == nil {
//					count += c
//				} else {
//					b.logger.Error("join channel from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
//				}
//			})
//			return true
//		})
//		wg.Wait()
//		return count, nil
//	}
//
//	func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) (count int32, err error) {
//		count = b.leaveChannel(uid, channels)
//		wg := sync.WaitGroup{}
//		b.peers.Range(func(id string, peer *peerClient) bool {
//			wg.Go(func() {
//				if c, err := peer.leaveChannel(ctx, uid, channels); err == nil {
//					count += c
//				} else {
//					b.logger.Error("leave channel from peer failed", xlog.Peer(id), xlog.Uid(uid), xlog.Err(err))
//				}
//			})
//			return true
//		})
//		wg.Wait()
//		return count, nil
//	}
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.requstHandlers.Set(method, handler)
}
func (b *broker) userOpened(req *protos.UserOpenedReq) {

}
func (b *broker) userClosed(req *protos.UserClosedReq) {

}
func (b *broker) onUserClosed(id *packet.Identity) {
	b.userClients.DeleteKey(id.UserID, id.ClientID)
	b.clientChannels.RangeKey(id.ClientID, func(channel string, _ struct{}) bool {
		b.channelClients.DeleteKey(channel, id.ClientID)
		return true
	})
	b.clientChannels.Delete(id.ClientID)
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
	b.userClients.SetKey(p.Identity.UserID, p.Identity.ClientID, idAndNet{id: b.id, net: string(net)})
	if chs, err := b.handler.GetChannels(p.Identity.UserID); err == nil {
		for _, ch := range chs {
			b.clientChannels.SetKey(p.Identity.ClientID, ch, struct{}{})
			b.channelClients.SetKey(ch, p.Identity.ClientID, idAndNet{id: b.id, net: string(net)})
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

//	func (b *broker) isOnline(uid string) (ok bool) {
//		b.userClients.RangeKey(uid, func(cid string, net server.Transport) bool {
//			if b.isActive(cid, net) {
//				ok = true
//				return false
//			}
//			return true
//		})
//		return ok
//	}
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
	// b.userClients.RangeKey(uid, func(cid string, net server.Transport) bool {
	// 	total++
	// 	l, ok := b.listeners[net]
	// 	if !ok {
	// 		return true
	// 	}
	// 	if err := l.SendMessage(ctx, cid, m); err != nil {
	// 		b.logger.Error("send message to user failed", xlog.Uid(uid), xlog.Err(err))
	// 	} else {
	// 		success++
	// 	}
	// 	return true
	// })
	return total, success
}

// func (b *broker) sendToChannel(ctx context.Context, channel string, m *packet.Message, targets []*protos.Target) (total, success int32) {
// 	b.channelClients.RangeKey(channel, func(cid string, net server.Transport) bool {
// 		total++
// 		if l, ok := b.listeners[net]; ok {
// 		if !ok {
// 			return true
// 		}
// 		if err := l.SendMessage(ctx, cid, m); err != nil {
// 			b.logger.Error("send message to channel failed", xlog.Channel(channel), xlog.Err(err))
// 		} else {
// 			success++
// 		}
// 		return true
// 	})
// 	return total, success
// }

// func (b *broker) joinChannel(uid string, channels []string) (count int32) {
// 	b.userClients.RangeKey(uid, func(cid string, net server.Transport) bool {
// 		count++
// 		for _, ch := range channels {
// 			b.channelClients.SetKey(ch, cid, net)
// 			b.clientChannels.SetKey(cid, ch, struct{}{})
// 		}
// 		return true
// 	})
// 	return count
// }
// func (b *broker) leaveChannel(uid string, channels []string) (count int32) {
// 	b.userClients.RangeKey(uid, func(cid string, net server.Transport) bool {
// 		count++
// 		for _, ch := range channels {
// 			b.channelClients.DeleteKey(ch, cid)
// 			b.clientChannels.DeleteKey(cid, ch)
// 		}
// 		return true
// 	})
// 	return count
// }
