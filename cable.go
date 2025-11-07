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
	case "tcp":
		if options.UseNIO {
			return nio.NewNIO(address, options)
		}
		return tcp.NewTCP(address, options)
	case "quic":
		return quic.NewQUIC(address, options)
	default:
		panic(fmt.Sprintf("Network type %s not support!", options.Network))
	}
}
