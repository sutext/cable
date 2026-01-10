package cluster

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Listener struct {
	Address string
	Network string
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
	handler    Handler
	brokerID   uint64
	httpPort   string
	peerPort   string
	initSize   int32
	listeners  map[string]string
	quicConfig *quic.Config
}

func newOptions(opts ...Option) *options {
	options := &options{
		handler:  &emptyHandler{},
		httpPort: ":8888",
		peerPort: ":4567",
		brokerID: getBrokerID() + 10000,
		initSize: 3,
		listeners: map[string]string{
			server.NetworkTCP:       ":1683",
			server.NetworkUDP:       ":1684",
			server.NetworkQUIC:      ":1685",
			server.NetworkWebSocket: ":1688",
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
func WithListener(net string, addr string) Option {
	return Option{func(o *options) {
		o.listeners[net] = addr
	}}
}

func WithBrokerID(id uint64) Option {
	return Option{func(o *options) {
		o.brokerID = id
	}}
}
func WithInitSize(size int32) Option {
	return Option{func(o *options) {
		o.initSize = size
	}}
}

// WithQuicConfig sets the QUIC configuration for the broker.
func WithQuicConfig(config *quic.Config) Option {
	return Option{func(o *options) {
		o.quicConfig = config
	}}
}

// getBrokerID returns a unique ID for the broker.
func getBrokerID() uint64 {
	if str := os.Getenv("BROKER_ID"); str != "" {
		if id, err := strconv.ParseUint(str, 10, 64); err == nil {
			return id
		}
	}
	if hostname, err := os.Hostname(); err == nil {
		strs := strings.Split(hostname, "-")
		if len(strs) > 1 {
			if id, err := strconv.ParseUint(strs[len(strs)-1], 10, 64); err == nil {
				return id
			}
		}
	}
	return rand.Uint64()
}
