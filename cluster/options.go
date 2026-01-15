package cluster

import (
	"crypto/tls"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/quic"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

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
	handler   Handler
	brokerID  uint64
	httpPort  string
	peerPort  string
	initSize  int32
	listeners []*Listener
}

func newOptions(opts ...Option) *options {
	options := &options{
		handler:  &emptyHandler{},
		httpPort: ":8888",
		peerPort: ":4567",
		brokerID: getBrokerID() + 10000,
		initSize: 3,
		listeners: []*Listener{
			NewListener(server.NetworkWS, ":1881", true),
			NewListener(server.NetworkWSS, ":1882", false, server.WithTLSConfig(&tls.Config{})),
			NewListener(server.NetworkTCP, ":1883", true),
			NewListener(server.NetworkTLS, ":1884", false, server.WithTLSConfig(&tls.Config{})),
			NewListener(server.NetworkUDP, ":1885", true),
			NewListener(server.NetworkQUIC, ":1886", false, server.WithQUICConfig(&quic.Config{})),
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

func WithListeners(listerners ...*Listener) Option {
	return Option{func(o *options) {
		o.listeners = listerners
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
