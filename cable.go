package cable

import (
	"sutext.github.io/cable/broker"
	"sutext.github.io/cable/internal/server/quic"
	"sutext.github.io/cable/internal/server/tcp"
	"sutext.github.io/cable/internal/server/tcp/nio"
	"sutext.github.io/cable/server"
)

func NewServer(opts ...server.Option) (server.Server, error) {
	options := server.NewOptions(opts...)
	switch options.Network {
	case "tcp":
		if options.UseNIO {
			return nio.NewNIO(options), nil
		}
		return tcp.NewTCP(options), nil
	case "quic":
		return quic.NewQUIC(options), nil
	default:
		return nil, server.ErrNetworkNotSupport
	}
}
func NewBroker(config *broker.Config) broker.Broker {
	return broker.New(config)
}
