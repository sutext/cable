package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"golang.org/x/net/quic"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/stats"
	"sutext.github.io/cable/xlog"
)

type booter struct {
	brokerID string
	redis    *redisClient
	config   *config
	broker   cluster.Broker
	kafka    sarama.Client
	grpcApi  *grpcServer
	httpApi  *httpServer
	producer sarama.SyncProducer
	stats    *statistics
}

func bool2byte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
func newBooter(config *config) *booter {
	b := &booter{config: config}
	// kafka := sarama.NewClient(config.KafkaBrokers, sarama.NewConfig())
	liss := make([]*cluster.Listener, len(config.Listeners))
	for i, l := range config.Listeners {
		var tlsConfig *tls.Config
		var quicConfig *quic.Config
		if l.TLS != nil {
			cert, err := tls.LoadX509KeyPair(l.TLS.CertFile, l.TLS.KeyFile)
			if err != nil {
				xlog.Error("Failed to load TLS certificate", xlog.Err(err))
			} else {
				tlsConfig = &tls.Config{
					Certificates: []tls.Certificate{cert},
				}
				quicConfig = &quic.Config{
					TLSConfig: tlsConfig,
				}
			}
		}
		lis := cluster.NewListener(l.Network, l.Port, l.AutoStart,
			server.WithSendQueue(l.QueueSize),
			server.WithTLSConfig(tlsConfig),
			server.WithQUICConfig(quicConfig),
		)
		liss[i] = lis
	}
	var grpcHandler stats.GrpcHandler
	if b.config.Trace.Enabled || b.config.Metrics.Enabled {
		grpcHandler = stats.GrpcHandler{
			Client: otelgrpc.NewClientHandler(),
			Server: otelgrpc.NewServerHandler(),
		}

	}
	b.stats = newStats(config)
	b.broker = cluster.NewBroker(
		cluster.WithBrokerID(b.config.BrokerID),
		cluster.WithHandler(b),
		cluster.WithClusterSize(config.ClusterSize),
		cluster.WithListeners(liss),
		cluster.WithStatsHandler(b.stats),
		cluster.WithGrpcStatsHandler(grpcHandler),
	)

	b.brokerID = fmt.Sprintf("%d", b.broker.ID())
	b.grpcApi = newGRPC(b)
	b.httpApi = newHTTP(b)
	return b
}
func (b *booter) Start() {
	b.redis = newRedis(b.config, b.stats)
	b.startKafka()
	go func() {
		if err := b.grpcApi.Serve(); err != nil {
			xlog.Error("Failed to start API server", xlog.Err(err))
		}
	}()
	go func() {
		if err := b.httpApi.Serve(); err != nil {
			xlog.Error("Failed to start HTTP server", xlog.Err(err))
		}
	}()
	b.broker.Start()
}
func (b *booter) startKafka() {
	if !b.config.Kafka.Enabled {
		return
	}
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.Return.Successes = true
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	kafkaConfig.Producer.Partitioner = sarama.NewRandomPartitioner
	var err error
	b.kafka, err = sarama.NewClient(b.config.Kafka.Brokers, kafkaConfig)
	if err != nil {
		xlog.Error("Failed to initialize Kafka client", xlog.Err(err))
	} else {
		b.producer, err = sarama.NewSyncProducerFromClient(b.kafka)
		if err != nil {
			xlog.Error("Failed to initialize Kafka producer", xlog.Err(err))
		}
	}
}

func (b *booter) Shutdown(ctx context.Context) error {
	if b.redis != nil {
		b.redis.Close()
	}
	if b.producer != nil {
		b.producer.Close()
	}
	if b.kafka != nil {
		b.kafka.Close()
	}
	b.stats.Shutdown(ctx)
	return b.broker.Shutdown(ctx)
}

// -- Handler methods --
func (b *booter) OnUserConnect(ctx context.Context, p *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}
func (b *booter) OnUserClosed(id *packet.Identity) {

}
func (b *booter) OnUserMessage(ctx context.Context, m *packet.Message, id *packet.Identity) error {
	route, ok := b.config.MessageRoute[m.Kind]
	if !ok || !route.Enabled {
		return nil
	}
	wg := &sync.WaitGroup{}
	toUserID := ""
	toChannel := ""
	switch route.ResendType {
	case resendToAll:
		wg.Go(func() {
			go b.SendToAll(ctx, m)
		})
	case resendToUser:
		uid, ok := m.Get(packet.PropertyUserID)
		if !ok {
			// No user ID in message, do nothing
			return nil
		}
		toUserID = uid
		wg.Go(func() {
			b.SendToUser(ctx, uid, m)
		})
	case resendToChannel:
		channel, ok := m.Get(packet.PropertyChannel)
		if !ok {
			return nil
		}
		toChannel = channel
		wg.Go(func() {
			b.SendToChannel(ctx, channel, m)
		})
	case resendNone:
		// Do not resend, just process Kafka
	default:
		// Invalid resendType, do nothing
		xlog.Warn("Invalid resendType in message route",
			xlog.Str("resendType", string(route.ResendType)),
			xlog.Int("kind", int(m.Kind)))
	}
	// Send message to Kafka if topic is configured
	if route.KafkaTopic != "" && b.producer != nil {
		wg.Go(func() {
			b.SendToKafka(ctx, route.KafkaTopic, toUserID, toChannel, id, m)
		})
	}
	wg.Wait()
	return nil
}

func (b *booter) GetUserChannels(uid string) (map[string]string, error) {
	return b.ListChannels(context.Background(), uid)
}

func (b *booter) userKey(uid string) string {
	return fmt.Sprintf("%s:%s", b.config.Redis.UserPrefix, uid)
}
func (b *booter) ListChannels(ctx context.Context, uid string) (map[string]string, error) {
	if b.redis == nil {
		return nil, nil
	}
	return b.redis.HGetAll(ctx, b.userKey(uid)).Result()
}
func (b *booter) JoinChannel(ctx context.Context, uid string, channels map[string]string) error {
	if b.redis == nil {
		return nil
	}
	_, err := b.redis.HSet(ctx, b.userKey(uid), "channels", channels).Result()
	if err != nil {
		return err
	}
	return b.broker.JoinChannel(ctx, uid, channels)

}
func (b *booter) LeaveChannel(ctx context.Context, uid string, channels map[string]string) error {
	if b.redis == nil {
		return nil
	}
	_, err := b.redis.HSet(ctx, b.userKey(uid), "channels", channels).Result()
	if err != nil {
		return err
	}
	return b.broker.LeaveChannel(ctx, uid, channels)
}

func (b *booter) SendToAll(ctx context.Context, m *packet.Message) (int32, int32, error) {
	kindStr := fmt.Sprintf("%d", m.Kind)
	total, success, err := b.broker.SendToAll(ctx, m)
	if err != nil {
		xlog.Error("Failed to send message to all", xlog.Err(err))
	} else {
		xlog.Debug("Sent message to all clients",
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Str("kind", kindStr))
	}
	return total, success, err
}
func (b *booter) SendToUser(ctx context.Context, uid string, m *packet.Message) (int32, int32, error) {
	kindStr := fmt.Sprintf("%d", m.Kind)
	total, success, err := b.broker.SendToUser(ctx, uid, m)
	if err != nil {
		xlog.Error("Failed to send message to user",
			xlog.Err(err),
			xlog.Uid(uid),
			xlog.Str("kind", kindStr))
	} else {
		xlog.Info("Sent message to user",
			xlog.Uid(uid),
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Str("kind", kindStr))
	}
	return total, success, err
}
func (b *booter) SendToChannel(ctx context.Context, channel string, m *packet.Message) (int32, int32, error) {
	kindStr := fmt.Sprintf("%d", m.Kind)
	total, success, err := b.broker.SendToChannel(ctx, channel, m)
	if err != nil {
		xlog.Error("Failed to send message to channel",
			xlog.Err(err),
			xlog.Str("channel", channel),
			xlog.Str("kind", kindStr))
	} else {
		xlog.Info("Sent message to channel",
			xlog.Str("channel", channel),
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Str("kind", kindStr))
	}
	return total, success, err
}
func (b *booter) SendToKafka(ctx context.Context, topic, toUserID, toChannel string, id *packet.Identity, m *packet.Message) {
	var err error
	var offset int64
	var partition int32
	startTime := time.Now()
	ctx = b.stats.KafkaBegin(ctx, &KafkaBegin{
		BeginTime: startTime,
		Topic:     topic,
	})
	defer func() {
		b.stats.KafkaEnd(ctx, &KafkaEnd{
			EndTime: time.Now(),
			Topic:   topic,
			Error:   err,
		})
	}()
	headers := &KafkaHeader{
		raw: []sarama.RecordHeader{
			{Key: []byte("messageQos"), Value: []byte{byte(m.Qos)}},
			{Key: []byte("messageDup"), Value: []byte{bool2byte(m.Dup)}},
			{Key: []byte("messageKind"), Value: []byte{byte(m.Kind)}},
		},
	}
	if toUserID != "" {
		headers.Set("toUser", toUserID)
	}
	if toChannel != "" {
		headers.Set("toChannel", toChannel)
	}
	if id != nil {
		headers.Set("fromUser", id.UserID)
		headers.Set("fromClient", id.ClientID)
	}
	otel.GetTextMapPropagator().Inject(ctx, headers)
	kafkaMsg := &sarama.ProducerMessage{
		Topic:   topic,
		Value:   sarama.ByteEncoder(m.Payload),
		Headers: headers.raw,
	}
	partition, offset, err = b.producer.SendMessage(kafkaMsg)
	if err != nil {
		xlog.Error("Failed to send message to Kafka",
			xlog.Err(err),
			xlog.Str("topic", topic),
			xlog.Int("kind", int(m.Kind)))
	} else {
		xlog.Debug("Sent message to Kafka",
			xlog.Str("topic", topic),
			xlog.I32("partition", partition),
			xlog.I64("offset", offset),
			xlog.Int("kind", int(m.Kind)))
	}
}
