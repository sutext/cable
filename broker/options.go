package broker

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strings"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Listener struct {
	Address string
	Network server.Transport
}
type Handler interface {
	OnClosed(id *packet.Identity)
	OnConnect(p *packet.Connect) (code packet.ConnectCode)
	OnMessage(p *packet.Message, id *packet.Identity) error
	GetChannels(uid string) (channels []string, err error) //uid join channels
}
type emptyHandler struct{}

func (h *emptyHandler) OnClosed(id *packet.Identity) {
}
func (h *emptyHandler) OnConnect(p *packet.Connect) (code packet.ConnectCode) {
	return packet.ConnectAccepted
}
func (h *emptyHandler) OnMessage(p *packet.Message, id *packet.Identity) error {
	return nil
}
func (h *emptyHandler) GetChannels(uid string) (channels []string, err error) {
	return nil, nil
}

type options struct {
	peers     []string
	handler   Handler
	brokerID  string
	httpPort  string
	peerPort  string
	listeners map[server.Transport]string
}

func newOptions(opts ...Option) *options {
	options := &options{
		handler:  &emptyHandler{},
		httpPort: ":8888",
		peerPort: ":4567",
		brokerID: getBrokerID(),
		listeners: map[server.Transport]string{
			server.TransportTCP:  ":1883",
			server.TransportUDP:  ":1884",
			server.TransportGRPC: ":1885",
		},
	}
	for _, opt := range opts {
		opt.f(options)
	}
	return options
}

type Option struct {
	f func(*options)
}

// WithPeers sets the list of peers to connect to.
// The list should contain the addresses of the other brokers in the cluster.
// eg []string{"broker1@127.0.0.1:8080", "broker2@127.0.0.1:8081"}
func WithPeers(peers []string) Option {
	return Option{func(o *options) {
		o.peers = peers
	}}
}
func WithHandler(h Handler) Option {
	return Option{func(o *options) {
		o.handler = h
	}}
}

func WithHTTPPort(port string) Option {
	return Option{func(o *options) {
		o.httpPort = port
	}}
}
func WithPeerPort(port string) Option {
	return Option{func(o *options) {
		o.peerPort = port
	}}
}

// WithListener sets the listener for the broker.
func WithListener(net server.Transport, addr string) Option {
	return Option{func(o *options) {
		o.listeners[net] = addr
	}}
}

func WithBrokerID(id string) Option {
	return Option{func(o *options) {
		o.brokerID = id
	}}
}
func getBrokerID() string {
	if id := os.Getenv("BROKER_ID"); id != "" {
		return id
	}
	if hostname, err := os.Hostname(); err == nil {
		strs := strings.Split(hostname, "-")
		if len(strs) > 1 {
			return fmt.Sprintf("cable%s", strs[len(strs)-1])
		}
		return hostname
	}
	return fmt.Sprintf("cable%d", rand.Int())
}
