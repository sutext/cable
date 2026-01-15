package cluster

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"sutext.github.io/cable/packet"
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
	httpPort  uint16
	peerPort  uint16
	initSize  int32
	listeners []*Listener
}

func newOptions(opts ...Option) *options {
	options := &options{
		handler:  &emptyHandler{},
		httpPort: 8888,
		peerPort: 4567,
		initSize: 3,
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

func WithHTTPPort(port uint16) Option {
	return Option{func(o *options) {
		o.httpPort = port
	}}
}
func WithPeerPort(port uint16) Option {
	return Option{func(o *options) {
		o.peerPort = port
	}}
}

func WithListeners(listerners []*Listener) Option {
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
	if hostname, err := os.Hostname(); err == nil {
		strs := strings.Split(hostname, "-")
		if len(strs) > 1 {
			if id, err := strconv.ParseUint(strs[len(strs)-1], 10, 64); err == nil {
				return id + 10000
			}
		}
	}
	return rand.Uint64()
}
