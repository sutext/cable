package broker

import (
	"fmt"
	"math/rand/v2"
	"net"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Listener struct {
	Address string
	Network server.Network
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
	httpAddr  string
	listeners map[server.Network]string
}

func newOptions(opts ...Option) *options {
	ip, err := getLocalIP()
	if err != nil {
		panic(err)
	}
	options := &options{
		handler:  &emptyHandler{},
		httpAddr: ":8888",
		brokerID: fmt.Sprintf("%d@%s:4567", rand.Int64(), ip),
		listeners: map[server.Network]string{
			server.NetworkTCP: ":1883",
			server.NetworkUDP: ":1884",
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

func WithHTTPAddr(addr string) Option {
	return Option{func(o *options) {
		o.httpAddr = addr
	}}
}

// WithListener sets the listener for the broker.
func WithListener(net server.Network, addr string) Option {
	return Option{func(o *options) {
		o.listeners[net] = addr
	}}
}

func WithBrokerID(id string) Option {
	return Option{func(o *options) {
		o.brokerID = id
	}}
}

// getLocalIP retrieves the non-loopback local IPv4 address of the machine.
func getLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		// Check if the interface is up and not a loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no network interface found")
}
