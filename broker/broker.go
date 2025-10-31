package broker

import (
	"context"

	"sutext.github.io/cable/api/kitex_gen/api"
	"sutext.github.io/cable/api/kitex_gen/api/service"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/internal/server/tcp/nio"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

type Broker interface {
	api.Service
	Start() error
	Shutdown() error
}

type broker struct {
	peers      *safe.Map[string, *peerClient]
	config     *Config
	channels   *safe.Map[string, *safe.Set[string]]
	taskQueue  *queue.Queue
	userServer server.Server
	peerServer *peerServer
}

func New(config *Config) Broker {
	return &broker{
		config:    config,
		peers:     safe.NewMap[string, *peerClient](),
		taskQueue: queue.NewQueue(10, 20),
		channels:  safe.NewMap[string, *safe.Set[string]](),
	}
}

func (b *broker) Start() error {
	b.peerServer = &peerServer{broker: b}
	svr := service.NewServer(b.peerServer)

	err := svr.Run()

	if err != nil {
		return err
	}

	b.userServer = nio.NewNIO(server.NewOptions(server.WithTCP(b.config.Addr)))
	b.userServer.OnAuth(func(p *packet.Identity) error {
		return nil
	})
	b.userServer.OnData(func(uid string, p *packet.DataPacket) (*packet.DataPacket, error) {
		return nil, nil
	})

	return b.userServer.Serve()
}

func (b *broker) Shutdown() error {
	return b.userServer.Shutdown(context.Background())
}
func (b *broker) join(channels []string, uid string) {
	for _, channel := range channels {
		if users, ok := b.channels.Get(channel); ok {
			users.Add(uid)
		} else {
			b.channels.Set(channel, safe.NewSet(uid))
		}
	}
}
func (b *broker) leave(channels []string, uid string) {
	for _, channel := range channels {
		if users, ok := b.channels.Get(channel); ok {
			users.Del(uid)
		}
	}
}
func (b *broker) publish(channel string, payload []byte) {
	if users, ok := b.channels.Get(channel); ok {
		users.Range(func(uid string) bool {
			b.taskQueue.Push(func() {
				if conn, err := b.userServer.GetConn(uid); err != nil {
					conn.SendData(payload)
				}
			})
			return true
		})
	}
}
func (b *broker) Join(ctx context.Context, req *api.JoinRequest) (r *api.JoinResponse, err error) {
	b.join(req.Channels, req.Uid)
	b.peers.Range(func(key string, peer *peerClient) bool {
		peer.Join(ctx, req.Channels, req.Uid)
		return true
	})

	return api.NewJoinResponse(), nil
}

func (b *broker) Leave(ctx context.Context, req *api.JoinRequest) (r *api.JoinResponse, err error) {
	b.leave(req.Channels, req.Uid)
	b.peers.Range(func(key string, peer *peerClient) bool {
		peer.Leave(ctx, req.Channels, req.Uid)
		return true
	})
	return api.NewJoinResponse(), nil
}

func (b *broker) Publish(ctx context.Context, req *api.PublishRequest) (err error) {
	b.publish(req.Channel, req.Payload)
	b.peers.Range(func(key string, peer *peerClient) bool {
		peer.Publish(ctx, req.Channel, req.Payload)
		return true
	})
	return nil
}
