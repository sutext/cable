package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"sutext.github.io/cable"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Broker interface {
	Start() error
	Shutdown() error
	Inspects() ([]*Inspect, error)
	IsOnline(ctx context.Context, uid string) (ok bool)
	KickConn(ctx context.Context, cid string)
	KickUser(ctx context.Context, uid string)
	SendMessage(ctx context.Context, m *packet.Message) error
}
type broker struct {
	id           string
	peers        []*peer
	users        KeyMap
	logger       logger.Logger
	channels     KeyMap
	listeners    map[uint8]server.Server
	peerServer   server.Server
	inpectServer *http.Server
}

func NewBroker(opts ...Option) Broker {
	options := newOptions(opts...)
	b := &broker{id: options.brokerID}
	b.logger = options.logger
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
	b.listeners = make(map[uint8]server.Server, len(options.listeners))
	for _, l := range options.listeners {
		b.listeners[l.Network] = cable.NewServer(l.Address,
			server.WithConnect(b.onUserConnect),
			server.WithMessage(b.onUserMessage),
			server.WithLogger(options.logger),
		)
	}
	b.peerServer = cable.NewServer(strings.Split(b.id, "@")[1], server.WithRequest(b.onPeerRequest))
	b.inpectServer = &http.Server{Addr: ":8888", Handler: b}
	return b
}
func (b *broker) Start() error {
	for _, l := range b.listeners {
		go l.Serve()
	}
	go b.peerServer.Serve()
	time.AfterFunc(time.Second*5, func() {
		for _, p := range b.peers {
			p.Connect()
		}
	})
	go b.inpectServer.ListenAndServe()
	return nil
}
func (b *broker) Shutdown() error {
	ctx := context.Background()
	for _, l := range b.listeners {
		l.Shutdown(ctx)
	}
	return b.peerServer.Shutdown(ctx)
}
func (b *broker) Inspects() ([]*Inspect, error) {
	ctx := context.Background()
	inspects := make([]*Inspect, len(b.peers)+2)
	inspects[0] = NewInspect()
	inspects[1] = b.inspect()
	for i, p := range b.peers {
		isp, err := p.inspect(ctx)
		if err != nil {
			return nil, err
		}
		inspects[0].merge(isp)
		inspects[i+2] = isp
	}
	return inspects, nil
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
func (b *broker) SendMessage(ctx context.Context, m *packet.Message) (t, s int, e error) {
	total, success := b.sendMessage(m)
	for _, p := range b.peers {
		t, s, e := p.sendMessage(ctx, m)
		if e != nil {
			return 0, 0, e
		}
		total += t
		success += s
	}
	return total, success, nil
}
func (b *broker) JoinChannel(uid string, channels ...string) error {
	return b.JoinChannels(uid, channels)
}
func (b *broker) JoinChannels(uid string, channels []string) error {
	for _, c := range channels {
		b.users.Range(uid, func(cid string, net uint8) bool {
			b.channels.Set(c, cid, net)
			return true
		})
	}
	return nil
}
func (b *broker) LeaveChannel(uid string, channels ...string) error {
	return b.LeaveChannels(uid, channels)
}
func (b *broker) LeaveChannels(uid string, channels []string) error {
	for _, c := range channels {
		b.users.Range(uid, func(cid string, net uint8) bool {
			b.channels.Delete(c, cid)
			return true
		})
	}
	return nil
}
func (b *broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isps, err := b.Inspects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(isps, "", "\t")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (b *broker) onUserMessage(s server.Server, p *packet.Message, id *packet.Identity) error {
	total, success, err := b.SendMessage(context.Background(), p)
	if err != nil {
		b.logger.Error("Failed to send message to broker", "error", err, "broker", b.id)
	} else {
		b.logger.Info("Sent message to broker", "total", total, "success", success)
	}
	return err
}
func (b *broker) onUserConnect(s server.Server, p *packet.Connect) packet.ConnackCode {
	if net, ok := b.users.Get(p.Identity.UserID, p.Identity.ClientID); ok {
		if l, ok := b.listeners[net]; ok {
			l.KickConn(p.Identity.ClientID)
		}
	}
	b.users.Set(p.Identity.UserID, p.Identity.ClientID, s.Network())
	chs := []string{"news", "sports", "tech", "music", "movies"}
	b.channels.Set("all", p.Identity.ClientID, s.Network())
	b.channels.Set(chs[rand.IntN(len(chs))], p.Identity.ClientID, s.Network())
	return packet.ConnectionAccepted
}
func (b *broker) onPeerRequest(s server.Server, p *packet.Request, id *packet.Identity) (*packet.Response, error) {
	switch p.Path {
	case "SendMessage":
		msg := &packet.Message{}
		if err := packet.Unmarshal(msg, p.Body); err != nil {
			return nil, err
		}
		total, success := b.sendMessage(msg)
		return packet.NewResponse(p.Seq, []byte(fmt.Sprintf("%d/%d", success, total))), nil
	case "IsOnline":
		online := b.isOnline(string(p.Body))
		var r byte
		if online {
			r = 1
		}
		return packet.NewResponse(p.Seq, []byte{r}), nil
	case "KickConn":
		b.kickConn(string(p.Body))
		return packet.NewResponse(p.Seq), nil
	case "KickUser":
		b.kickUser(string(p.Body))
		return packet.NewResponse(p.Seq), nil
	case "Inspect":
		isp := b.inspect()
		data, err := json.Marshal(isp)
		if err != nil {
			return nil, err
		}
		return packet.NewResponse(p.Seq, data), nil
	}
	return nil, fmt.Errorf("unsupported request method: %s", p.Path)
}
func (b *broker) isOnline(uid string) (ok bool) {
	b.users.Range(uid, func(cid string, net uint8) bool {
		l, ok := b.listeners[net]
		if !ok {
			return true
		}
		if conn, err := l.GetConn(cid); err == nil {
			if conn.IsActive() {
				ok = true
				return false
			}
		}
		return true
	})
	return ok
}
func (b *broker) kickConn(cid string) {
	for _, l := range b.listeners {
		l.KickConn(cid)
	}
}
func (b *broker) kickUser(uid string) {
	b.users.Range(uid, func(cid string, net uint8) bool {
		if l, ok := b.listeners[net]; ok {
			l.KickConn(cid)
		}
		return true
	})
}
func (b *broker) sendMessage(m *packet.Message) (total int, success int) {
	b.channels.Range(m.Channel, func(cid string, net uint8) bool {
		total++
		l, ok := b.listeners[net]
		if !ok {
			b.logger.Error("Failed to find listener", "network", net)
			return true
		}
		if conn, err := l.GetConn(cid); err == nil {
			if err = conn.SendMessage(m); err != nil {
				b.logger.Error("Failed to send message to client", "error", err, "client", cid)
			} else {
				success++
			}
		}
		return true
	})
	return total, success
}
func (b *broker) inspect() *Inspect {
	return &Inspect{
		ID:       b.id,
		Clients:  len(b.users.Dump()),
		Channels: b.channels.Dump(),
	}
}
