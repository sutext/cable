package cable

import (
	"fmt"

	"sutext.github.io/cable/internal/server/quic"
	"sutext.github.io/cable/internal/server/tcp"
	"sutext.github.io/cable/internal/server/tcp/nio"
	"sutext.github.io/cable/server"
)

func NewServer(address string, opts ...server.Option) server.Server {
	options := server.NewOptions(opts...)
	switch options.Network {
	case server.NetworkTCP:
		if options.UseNIO {
			return nio.NewNIO(address, options)
		}
		return tcp.NewTCP(address, options)
	case server.NetworkQUIC:
		return quic.NewQUIC(address, options)
	default:
		panic(fmt.Sprintf("Transport type %s not support!", options.Network))
	}
}
