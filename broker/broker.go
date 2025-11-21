package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sutext.github.io/cable"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/keymap"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type BrokerID string
type Broker interface {
	Start() error
	Shutdown() error
	Inspects() ([]*Inspect, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickConn(ctx context.Context, cid string)
	KickUser(ctx context.Context, uid string)
	SendMessage(ctx context.Context, m *packet.Message) (total, success uint64, err error)
	JoinChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
	LeaveChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error)
}
type broker struct {
	id         string
	peers      []*peer
	users      keymap.KeyMap[server.Network]
	logger     logger.Logger
	handler    Handler
	clients    keymap.KeyMap[server.Network]
	channels   keymap.KeyMap[server.Network]
	listeners  map[server.Network]server.Server
	peerServer server.Server
	mux        *http.ServeMux
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.logger = options.logger
	b.handler = options.handler
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
	b.listeners = make(map[server.Network]server.Server, len(options.listeners))
	for _, l := range options.listeners {
		b.listeners[l.Network] = cable.NewServer(l.Address,
			server.WithClose(b.onUserClosed),
			server.WithConnect(func(p *packet.Connect) packet.ConnectCode {
				return b.onUserConnect(p, l.Network)
			}),
			server.WithMessage(b.onUserMessage),
			server.WithRequest(b.onUserRequest),
			server.WithLogger(options.logger),
		)
	}
	b.peerServer = cable.NewServer(strings.Split(b.id, "@")[1], server.WithRequest(b.onPeerRequest))
	b.mux = http.NewServeMux()
	b.mux.HandleFunc("/join", b.handleJoin)
	b.mux.HandleFunc("/inspect", b.handleInspect)
	b.mux.HandleFunc("/kickout", b.handleKickout)
	b.mux.HandleFunc("/message", b.handleMessage)
	return b
}
func (b *broker) Start() error {
	for _, l := range b.listeners {
		go l.Serve()
	}
	go b.peerServer.Serve()
	time.AfterFunc(time.Second*2, func() {
		for _, p := range b.peers {
			p.Connect()
		}
	})
	go http.ListenAndServe(":8888", b.mux)
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
		if ok, _ = p.isOnline(ctx, uid); ok {
			return true
		}
	}
	return false
}
func (b *broker) KickConn(ctx context.Context, cid string) {
	b.kickConn(cid)
	for _, p := range b.peers {
		p.kickConn(ctx, cid)
	}
}
func (b *broker) KickUser(ctx context.Context, uid string) {
	b.kickUser(uid)
	for _, p := range b.peers {
		p.kickUser(ctx, uid)
	}
}
func (b *broker) SendMessage(ctx context.Context, m *packet.Message) (total, success uint64, err error) {
	total, success = b.sendMessage(m)
	for _, p := range b.peers {
		if t, s, err := p.sendMessage(ctx, m); err == nil {
			total += t
			success += s
		} else {
			return total, success, err
		}
	}
	return total, success, nil
}
func (b *broker) JoinChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error) {
	count = b.joinChannel(uid, channels)
	for _, p := range b.peers {
		if c, err := p.joinChannel(ctx, uid, channels); err == nil {
			count += c
		}
	}
	return count, nil
}
func (b *broker) LeaveChannel(ctx context.Context, uid string, channels ...string) (count uint64, err error) {
	count = b.leaveChannel(uid, channels)
	for _, p := range b.peers {
		if c, err := p.leaveChannel(ctx, uid, channels); err == nil {
			count += c
		}
	}
	return count, nil
}
func (b *broker) JoinCluster(ctx context.Context, id BrokerID) {
	b.joinCluster(id)
	for _, p := range b.peers {
		p.joinCluster(ctx, id)
	}
}
func (b *broker) onUserClosed(id *packet.Identity) {
	b.users.DeleteKey(id.UserID, id.ClientID)
	b.channels.DeleteKey("all", id.ClientID)
	b.clients.RangeKey(id.ClientID, func(channel string, net server.Network) bool {
		b.channels.DeleteKey(channel, id.ClientID)
		return true
	})
	b.clients.Delete(id.ClientID)
	b.handler.OnClosed(id)
}
func (b *broker) onUserConnect(p *packet.Connect, net server.Network) packet.ConnectCode {
	code := b.handler.OnConnect(p)
	if code != packet.ConnectAccepted {
		return code
	}
	for _, peer := range b.peers { // kick old connections at other peers
		peer.kickConn(context.Background(), p.Identity.ClientID)
	}
	b.users.SetKey(p.Identity.UserID, p.Identity.ClientID, net)
	b.channels.SetKey("all", p.Identity.ClientID, net)
	if chs, err := b.handler.GetChannels(p.Identity.UserID); err == nil {
		for _, ch := range chs {
			b.clients.SetKey(p.Identity.ClientID, ch, net)
			b.channels.SetKey(ch, p.Identity.ClientID, net)
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
	case "JoinCluster":
		brokerID := BrokerID(p.Content)
		b.joinCluster(brokerID)
		return packet.NewResponse(p.ID), nil
	default:
		return nil, fmt.Errorf("unsupported request path: %s", p.Method)
	}
}
func (b *broker) isOnline(uid string) (ok bool) {
	b.users.RangeKey(uid, func(cid string, net server.Network) bool {
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
		if c, ok := l.GetConn(cid); ok {
			if c.IsActive() {
				return true
			}
		}
	}
	return false
}
func (b *broker) kickConn(cid string) {
	for _, l := range b.listeners {
		l.KickConn(cid)
	}
}
func (b *broker) kickUser(uid string) {
	b.users.RangeKey(uid, func(cid string, net server.Network) bool {
		if l, ok := b.listeners[net]; ok {
			l.KickConn(cid)
		}
		return true
	})
}
func (b *broker) sendMessage(m *packet.Message) (total, success uint64) {
	b.channels.RangeKey(m.Channel, func(cid string, net server.Network) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			b.logger.Error("Failed to find listener", "network", net)
			return true
		}
		if conn, ok := l.GetConn(cid); ok {
			if err := conn.SendMessage(context.Background(), m); err != nil {
				b.logger.Error("Failed to send message to client", "error", err, "client", cid)
			} else {
				success++
			}
		}
		return true
	})
	return total, success
}
func (b *broker) joinChannel(uid string, channels []string) (count uint64) {
	b.users.RangeKey(uid, func(cid string, net server.Network) bool {
		count++
		for _, ch := range channels {
			b.channels.SetKey(ch, cid, net)
			b.clients.SetKey(cid, ch, net)
		}
		return true
	})
	return count
}
func (b *broker) leaveChannel(uid string, channels []string) (count uint64) {
	b.users.RangeKey(uid, func(cid string, net server.Network) bool {
		count++
		for _, ch := range channels {
			b.channels.DeleteKey(ch, cid)
			b.clients.DeleteKey(cid, ch)
		}
		return true
	})
	return count
}
func (b *broker) joinCluster(id BrokerID) {
	p := newPeer(b.id, string(id), b.logger)
	b.peers = append(b.peers, p)
	p.Connect()
}
