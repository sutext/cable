// Package cluster provides a distributed cluster implementation for the cable protocol.
// It supports broker management, peer communication, and distributed message routing.
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"sutext.github.io/cable/cluster/pb"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

// Broker defines the interface for the cluster broker.
// It provides methods for managing clients, channels, and message routing.
type Broker interface {
	ID() uint64
	// Start starts the broker and all its components.
	//
	Start()
	// Cluster returns the underlying cluster instance.
	//
	// Returns:
	// - Cluster: The cluster instance
	Cluster() Cluster
	// Shutdown shuts down the broker and all its components.
	//
	// Parameters:
	// - ctx: Context for the shutdown operation
	//
	// Returns:
	// - error: Error if shutdown fails, nil otherwise
	Shutdown(ctx context.Context) error
	// Inspects returns the status of all brokers in the cluster.
	//
	// Parameters:
	// - ctx: Context for the inspect operation
	//
	// Returns:
	// - []*pb.Status: List of broker statuses
	// - error: Error if inspecting fails, nil otherwise
	Inspects(ctx context.Context) ([]*pb.Status, error)
	// IsOnline checks if a user is online in the cluster.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - uid: User ID to check
	//
	// Returns:
	// - bool: True if the user is online, false otherwise
	IsOnline(ctx context.Context, uid string) (ok bool)
	// KickUser kicks a user from all connections in the cluster.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - uid: User ID to kick
	KickUser(ctx context.Context, uid string)
	// SendToAll sends a message to all clients in the cluster.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - m: Message to send
	//
	// Returns:
	// - int32: Total number of clients attempted to send to
	// - int32: Number of successful sends
	// - error: Error if sending fails, nil otherwise
	SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error)
	// SendToUser sends a message to a specific user in the cluster.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - uid: User ID to send to
	// - m: Message to send
	//
	// Returns:
	// - int32: Total number of clients attempted to send to
	// - int32: Number of successful sends
	// - error: Error if sending fails, nil otherwise
	SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error)
	// JoinChannel adds a user to one or more channels.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - uid: User ID to add to channels
	// - channels: set of channels to join
	//
	// Returns:
	// - error: Error if joining fails, nil otherwise
	JoinChannel(ctx context.Context, uid string, channels map[string]string) error
	// LeaveChannel removes a user from one or more channels.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - uid: User ID to remove from channels
	// - channels: set of channels to leave
	//
	// Returns:
	// - error: Error if leaving fails, nil otherwise
	LeaveChannel(ctx context.Context, uid string, channels map[string]string) error
	// SendToChannel sends a message to all clients in a channel.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - channel: Channel to send to
	// - m: Message to send
	//
	// Returns:
	// - int32: Total number of clients attempted to send to
	// - int32: Number of successful sends
	// - error: Error if sending fails, nil otherwise
	SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error)
	// ListUsers lists all users connected to the broker.
	//
	// Parameters:
	// - ctx: Context for the operation
	// - brokerID: ID of the broker to list users from
	//
	// Returns:
	// - map[string]map[string]string: Map of user IDs to map of client IDs to network protocols
	// - error: Error if listing fails, nil otherwise
	ListUsers(ctx context.Context, brokerID uint64) (map[string]map[string]string, error)
	// HandleRequest registers a handler for a specific request method.
	//
	// Parameters:
	// - method: Request method to handle
	// - handler: Handler function for the request method
	//
	// Returns:
	// - error: Error if registration fails, nil otherwise
	HandleRequest(method string, handler server.RequestHandler)
}

// broker implements the Broker interface and manages client connections, channels, and message routing.
type broker struct {
	id             uint64                                    // Unique broker ID
	logger         *xlog.Logger                              // Logger for broker events
	cluster        *cluster                                  // Underlying cluster instance
	handler        Handler                                   // Event handler for broker events
	listeners      map[string]*Listener                      // Map of network listeners (network -> listener)
	peerServer     *peerServer                               // gRPC server for peer communication
	userClients    safe.RMap[uint64, *safe.KeyMap[string]]   // Maps broker ID to user clients (bid -> uid -> cid -> network)
	channelClients safe.RMap[uint64, *safe.KeyMap[string]]   // Maps broker ID to channel clients (bid -> channel -> cid -> network)
	clientChannels safe.RMap[uint64, *safe.KeyMap[struct{}]] // Maps broker ID to client channels (bid -> cid -> channel -> null)
	requstHandlers safe.RMap[string, server.RequestHandler]  // Maps request methods to handlers
}

// NewBroker creates a new broker instance with the given options.
//
// Parameters:
// - opts: Configuration options for the broker
//
// Returns:
// - Broker: A new broker instance
func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{}
	if options.brokerID > 0 {
		b.id = options.brokerID
	} else {
		b.id = getBrokerID()
	}
	b.logger = xlog.With("BROKER", b.id)
	b.handler = options.handler
	b.listeners = make(map[string]*Listener)
	for _, l := range options.listeners {
		l.AddOptions(
			server.WithClose(b.onUserClosed),
			server.WithLogger(b.logger),
			server.WithMessage(b.onUserMessage),
			server.WithRequest(b.onUserRequest),
			server.WithConnect(func(p *packet.Connect) packet.ConnectCode {
				return b.onUserConnect(p, l.network)
			}),
		)
		b.listeners[l.network] = l
	}
	b.cluster = newCluster(b, options.clusterSize, options.peerPort)
	b.peerServer = newPeerServer(b, fmt.Sprintf(":%d", options.peerPort), options.grpcStatsHandler)
	return b
}
func (b *broker) ID() uint64 {
	return b.id
}

// Start starts the broker and all its components.
//
// Returns:
// - error: Error if starting fails, nil otherwise
func (b *broker) Start() {
	for _, l := range b.listeners {
		if l.autoStart {
			l.Start()
		}
	}
	go func() {
		if err := b.peerServer.Serve(); err != nil {
			b.logger.Info("peer server stoped", xlog.Err(err))
		}
	}()
	b.cluster.Start()
}

// Cluster returns the underlying cluster instance.
//
// Returns:
// - Cluster: The cluster instance
func (b *broker) Cluster() Cluster {
	return b.cluster
}

// Shutdown shuts down the broker and all its components.
//
// Parameters:
// - ctx: Context for the shutdown operation
//
// Returns:
// - error: Error if shutdown fails, nil otherwise
func (b *broker) Shutdown(ctx context.Context) (err error) {
	for _, l := range b.listeners {
		if err = l.Shutdown(ctx); err != nil {
			b.logger.Error("listener shutdown", xlog.Err(err))
		}
	}
	if err = b.peerServer.Shutdown(ctx); err != nil {
		b.logger.Error("peer server shutdown", xlog.Err(err))
	}
	b.cluster.Stop()
	return err
}

// ExpelAllConns expels all connections from all listeners.
func (b *broker) ExpelAllConns() {
	for _, l := range b.listeners {
		l.ExpelAllConns()
	}
}

// Inspects returns the status of all brokers in the cluster.
// It collects status information from the local broker and all peer brokers.
//
// Parameters:
// - ctx: Context for the operation
//
// Returns:
// - []*pb.Status: List of status information for all brokers in the cluster
// - error: Error if inspecting any broker fails, nil otherwise
func (b *broker) Inspects(ctx context.Context) ([]*pb.Status, error) {
	ss := make([]*pb.Status, 0, b.cluster.size)
	ss = append(ss, b.inspect())
	wg := sync.WaitGroup{}
	b.cluster.RangePeers(func(id uint64, cli *peerClient) {
		wg.Go(func() {
			s, err := cli.inspect(ctx)
			if err != nil {
				b.logger.Error("inspect peer failed", xlog.Peer(id), xlog.Err(err))
				return
			}
			ss = append(ss, s)
		})
	})
	wg.Wait()
	return ss, nil
}

// IsOnline checks if a user is online in the cluster.
//
// Parameters:
// - ctx: Context for the operation
// - uid: User ID to check
//
// Returns:
// - bool: True if the user is online, false otherwise
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

// KickUser kicks a user from all connections in the cluster.
//
// Parameters:
// - ctx: Context for the operation
// - uid: User ID to kick
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

// SendToAll sends a message to all clients in the cluster.
//
// Parameters:
// - ctx: Context for the operation
// - m: Message to send
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
// - error: Error if sending fails, nil otherwise
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

// SendToUser sends a message to a specific user in the cluster.
//
// Parameters:
// - ctx: Context for the operation
// - uid: User ID to send to
// - m: Message to send
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
// - error: Error if sending fails, nil otherwise
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

// SendToChannel sends a message to all clients in a channel.
//
// Parameters:
// - ctx: Context for the operation
// - channel: Channel to send to
// - m: Message to send
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
// - error: Error if sending fails, nil otherwise
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

// JoinChannel adds a user to one or more channels.
//
// Parameters:
// - ctx: Context for the operation
// - uid: User ID to add to channels
// - channels: List of channels to join
//
// Returns:
// - error: Error if joining fails, nil otherwise
func (b *broker) JoinChannel(ctx context.Context, uid string, channels map[string]string) error {
	op := &joinChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.cluster.SubmitOperation(ctx, op)
}

// LeaveChannel removes a user from one or more channels.
//
// Parameters:
// - ctx: Context for the operation
// - uid: User ID to remove from channels
// - channels: List of channels to leave
//
// Returns:
// - error: Error if leaving fails, nil otherwise
func (b *broker) LeaveChannel(ctx context.Context, uid string, channels map[string]string) error {
	op := &leaveChannelOp{
		uid:      uid,
		channels: channels,
	}
	return b.cluster.SubmitOperation(ctx, op)
}
func (b *broker) ListUsers(ctx context.Context, brokerID uint64) (map[string]map[string]string, error) {
	userClients, ok := b.userClients.Get(brokerID)
	if !ok {
		return nil, xerr.PeerNotReady
	}
	users := make(map[string]map[string]string)
	userClients.Range(func(s string, m *safe.Map[string, string]) bool {
		users[s] = m.Load()
		return true
	})
	return users, nil
}

// HandleRequest registers a handler for a specific request method.
//
// Parameters:
// - method: Request method to handle
// - handler: Handler function for the request method
func (b *broker) HandleRequest(method string, handler server.RequestHandler) {
	b.requstHandlers.Set(method, handler)
}

// onUserClosed handles user disconnect events.
// It submits a user closed operation to the cluster and notifies the handler.
//
// Parameters:
// - id: Identity of the disconnected user
func (b *broker) onUserClosed(id *packet.Identity) {
	op := &userClosedOp{
		uid: id.UserID,
		cid: id.ClientID,
	}
	if err := b.cluster.SubmitOperation(context.Background(), op); err != nil {
		b.logger.Error("Failed to submit user disconnect op", xlog.Err(err))
	}
	b.handler.OnUserClosed(id)
}

// onUserConnect handles user connect events.
// It checks for duplicate connections, submits a user opened operation to the cluster,
// and returns a connect code.
//
// Parameters:
// - p: Connect packet from the user
// - net: Network protocol used by the connection
//
// Returns:
// - packet.ConnectCode: Connect result code
func (b *broker) onUserConnect(p *packet.Connect, net string) packet.ConnectCode {
	code := b.handler.OnUserConnect(p)
	if code != packet.ConnectAccepted {
		return code
	}
	b.userClients.Range(func(id uint64, value *safe.KeyMap[string]) bool {
		if id == b.id {
			return true
		}
		if cnet, ok := value.GetKey(p.Identity.UserID, p.Identity.ClientID); ok {
			b.logger.Info("duplicate connection detected, kick old connection",
				xlog.Uid(p.Identity.UserID),
				xlog.Cid(p.Identity.ClientID),
				xlog.U64("old_peer_id", id),
				xlog.Str("old_net", cnet),
				xlog.U64("new_peer_id", b.id),
				xlog.Str("new_net", net),
			)
			if peer, ok := b.cluster.GetPeer(id); ok {
				cids := map[string]string{p.Identity.ClientID: net}
				if err := peer.kickConn(context.Background(), cids); err != nil {
					b.logger.Error("kick old connection from peer failed", xlog.Peer(id), xlog.Err(err))
				}
			}
		}
		return true
	})
	chs, err := b.handler.GetUserChannels(p.Identity.UserID)
	if err != nil {
		b.logger.Error("get channels failed", xlog.Err(err))
	}
	op := &userOpenedOp{
		uid:      p.Identity.UserID,
		cid:      p.Identity.ClientID,
		net:      net,
		nodeID:   b.id,
		channels: chs,
	}
	if err := b.cluster.SubmitOperation(context.Background(), op); err != nil {
		b.logger.Error("Failed to submit user connect op", xlog.Err(err))
		return packet.ConnectRejected
	}
	return packet.ConnectAccepted
}

// onUserMessage handles user message events.
// It delegates to the broker's message handler.
//
// Parameters:
// - p: Message packet received
// - id: Identity of the user that sent the message
//
// Returns:
// - error: Error if message handling fails, nil otherwise
func (b *broker) onUserMessage(p *packet.Message, id *packet.Identity) error {
	return b.handler.OnUserMessage(p, id)
}

// onUserRequest handles user request events.
// It looks up the appropriate request handler and invokes it.
//
// Parameters:
// - p: Request packet received
// - id: Identity of the user that sent the request
//
// Returns:
// - *packet.Response: Response packet to send back to the user
// - error: Error if request handling fails, nil otherwise
func (b *broker) onUserRequest(p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	handler, ok := b.requstHandlers.Get(p.Method)
	if !ok {
		return nil, xerr.RequestHandlerNotFound
	}
	return handler(p, id)
}

// isActive checks if any of the given targets are active connections.
//
// Parameters:
// - targets: Map of client IDs to network protocols
//
// Returns:
// - bool: True if any target is active, false otherwise
func (b *broker) isActive(targets map[string]string) bool {
	for cid, net := range targets {
		if l, ok := b.listeners[net]; ok && l.ISStarted() {
			if l.IsActive(cid) {
				return true
			}
		}
	}
	return false
}

// inspect returns the status of the local broker.
// It includes information about user count, channel count, and Raft status.
//
// Returns:
// - *pb.Status: Status information of the local broker
func (b *broker) inspect() *pb.Status {
	userCount := make(map[uint64]int32)
	b.userClients.Range(func(key uint64, value *safe.KeyMap[string]) bool {
		userCount[key] = value.Len()
		return true
	})
	channelCount := make(map[uint64]int32)
	b.channelClients.Range(func(key uint64, value *safe.KeyMap[string]) bool {
		channelCount[key] = value.Len()
		return true
	})
	status, _ := b.cluster.Status()
	progress := make(map[uint64]*pb.RaftProgress)
	if status.Progress != nil {
		for id, p := range status.Progress {
			progress[id] = &pb.RaftProgress{
				Match: p.Match,
				Next:  p.Next,
				State: p.State.String(),
			}
		}
	}
	return &pb.Status{
		Id:           b.id,
		UserCount:    userCount,
		ClientCount:  b.clientChannels.Len(),
		ClusterSize:  b.cluster.size,
		ChannelCount: channelCount,
		RaftState:    status.RaftState.String(),
		RaftTerm:     status.Term,
		RaftLogSize:  b.cluster.RaftLogSize(),
		RaftApplied:  status.Applied,
		RaftProgress: progress,
	}
}

// kickConn kicks the given targets from their respective listeners.
//
// Parameters:
// - targets: Map of client IDs to network protocols
func (b *broker) kickConn(targets map[string]string) {
	for cid, net := range targets {
		if l, ok := b.listeners[net]; ok && l.ISStarted() {
			l.KickConn(cid)
		}
	}
}

// sendToAll sends a message to all local clients.
//
// Parameters:
// - ctx: Context for the operation
// - m: Message to send
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
func (b *broker) sendToAll(ctx context.Context, m *packet.Message) (total, success int32) {
	for _, l := range b.listeners {
		if l.ISStarted() {
			t, s, err := l.Brodcast(ctx, m)
			total += t
			success += s
			if err != nil {
				b.logger.Error("send message to all failed", xlog.Err(err))
			}
		}
	}
	return total, success
}

// sendToTargets sends a message to specific target clients.
//
// Parameters:
// - ctx: Context for the operation
// - m: Message to send
// - targets: Map of client IDs to network protocols
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
func (b *broker) sendToTargets(ctx context.Context, m *packet.Message, targets map[string]string) (total, success int32) {
	for cid, net := range targets {
		total++
		if l, ok := b.listeners[net]; ok && l.ISStarted() {
			if err := l.SendMessage(ctx, cid, m); err != nil {
				b.logger.Error("send message to cid failed", xlog.Cid(cid), xlog.Err(err))
			} else {
				success++
			}
		}
	}
	return total, success
}

// userOpenedOp processes user opened operations.
// It updates the user clients, channel clients, and client channels mappings.
//
// Parameters:
// - op: User opened operation
func (b *broker) userOpenedOp(op *userOpenedOp) {
	userClients, _ := b.userClients.GetOrSet(op.nodeID, &safe.KeyMap[string]{})
	userClients.SetKey(op.uid, op.cid, op.net)
	channelClients, _ := b.channelClients.GetOrSet(op.nodeID, &safe.KeyMap[string]{})
	clientChannels, _ := b.clientChannels.GetOrSet(op.nodeID, &safe.KeyMap[struct{}]{})
	for ch := range op.channels {
		clientChannels.SetKey(op.cid, ch, struct{}{})
		channelClients.SetKey(ch, op.cid, op.net)
	}
}

// userClosedOp processes user closed operations.
// It removes the user from the user clients, channel clients, and client channels mappings.
//
// Parameters:
// - op: User closed operation
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

// joinChannelOp processes join channel operations.
// It adds a user to the specified channels across all brokers.
//
// Parameters:
// - op: Join channel operation
func (b *broker) joinChannelOp(op *joinChannelOp) {
	b.userClients.Range(func(id uint64, userClients *safe.KeyMap[string]) bool {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		clientChannels, _ := b.clientChannels.GetOrSet(id, &safe.KeyMap[struct{}]{})
		userClients.RangeKey(op.uid, func(cid string, net string) bool {
			for channel := range op.channels {
				channelClients.SetKey(channel, cid, net)
				clientChannels.SetKey(cid, channel, struct{}{})
			}
			return true
		})
		return true
	})
}

// leaveChannelOp processes leave channel operations.
// It removes a user from the specified channels across all brokers.
//
// Parameters:
// - op: Leave channel operation
func (b *broker) leaveChannelOp(op *leaveChannelOp) {
	b.userClients.Range(func(id uint64, userClients *safe.KeyMap[string]) bool {
		channelClients, _ := b.channelClients.GetOrSet(id, &safe.KeyMap[string]{})
		clientChannels, _ := b.clientChannels.GetOrSet(id, &safe.KeyMap[struct{}]{})
		userClients.RangeKey(op.uid, func(cid string, net string) bool {
			for ch := range op.channels {
				channelClients.DeleteKey(ch, cid)
				clientChannels.DeleteKey(cid, ch)
			}
			return true
		})
		return true
	})
}

// makeSnapshot creates a snapshot of the cluster state.
// It includes user clients, channel clients, and client channels mappings.
//
// Returns:
// - []byte: Serialized snapshot data
// - error: Error if snapshot creation fails, nil otherwise
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

// recoverFromSnapshot recovers the cluster state from a snapshot.
// It restores user clients, channel clients, and client channels mappings.
//
// Parameters:
// - s: Snapshot data to recover from
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
