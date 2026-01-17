package main

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/quic"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type booter struct {
	broker   cluster.Broker
	config   *config
	redis    *redis.Client
	kafka    sarama.Client
	producer sarama.SyncProducer
}

func newBooter(config *config) *booter {
	b := &booter{
		config: config,
	}
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
	b.broker = cluster.NewBroker(
		cluster.WithBrokerID(b.config.BrokerID),
		cluster.WithHandler(b),
		cluster.WithInitSize(config.InitSize),
		cluster.WithListeners(liss),
	)
	return b
}
func (b *booter) Start() error {
	// Initialize Redis
	rconfg := b.config.Redis
	b.redis = redis.NewClient(&redis.Options{
		Addr:     rconfg.Address,
		Password: rconfg.Password, // no password set
		DB:       rconfg.DB,       // use default DB
	})

	// Initialize Kafka client if configured
	if len(b.config.KafkaBrokers) > 0 {
		kafkaConfig := sarama.NewConfig()
		kafkaConfig.Producer.Return.Successes = true
		kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
		kafkaConfig.Producer.Partitioner = sarama.NewRandomPartitioner
		var err error
		b.kafka, err = sarama.NewClient(b.config.KafkaBrokers, kafkaConfig)
		if err != nil {
			xlog.Error("Failed to initialize Kafka client", xlog.Err(err))
		} else {
			b.producer, err = sarama.NewSyncProducerFromClient(b.kafka)
			if err != nil {
				xlog.Error("Failed to initialize Kafka producer", xlog.Err(err))
			}
		}
	}

	return b.broker.Start()
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
	return b.broker.Shutdown(ctx)
}
func (b *booter) OnConnect(c *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}
func (b *booter) OnClosed(id *packet.Identity) {

}
func (b *booter) OnMessage(m *packet.Message, id *packet.Identity) error {
	// Get route configuration for this message kind
	route, exists := b.config.MessageRoute[m.Kind]
	if !exists {
		// No route configured, do nothing
		return nil
	}
	ctx := context.Background()
	toUserID := ""
	toChannel := ""
	// Handle message resend based on resendType
	switch route.ResendType {
	case resendToAll:
		// Send message to all clients asynchronously
		go func() {
			total, success, err := b.broker.SendToAll(ctx, m)
			if err != nil {
				xlog.Error("Failed to send message to all", xlog.Err(err))
			} else {
				xlog.Debug("Sent message to all clients",
					xlog.I32("total", total),
					xlog.I32("success", success),
					xlog.Any("kind", uint8(m.Kind)))
			}
		}()
	case resendToUser:
		uid, ok := m.Get(packet.PropertyUserID)
		if !ok {
			// No user ID in message, do nothing
			return nil
		}
		toUserID = uid
		// Get user ID from message properties or use the sender's user ID
		// Send message to specified user asynchronously
		go func() {
			total, success, err := b.broker.SendToUser(ctx, uid, m)
			if err != nil {
				xlog.Error("Failed to send message to user",
					xlog.Err(err),
					xlog.Uid(uid),
					xlog.Any("kind", uint8(m.Kind)))
			} else {
				xlog.Info("Sent message to user",
					xlog.Uid(uid),
					xlog.I32("total", total),
					xlog.I32("success", success),
					xlog.Any("kind", uint8(m.Kind)))
			}
		}()
	case resendToChannel:
		channel, ok := m.Get(packet.PropertyChannel)
		if !ok {
			return nil
		}
		toChannel = channel
		// Send message to specified channel asynchronously
		go func() {
			total, success, err := b.broker.SendToChannel(ctx, channel, m)
			if err != nil {
				xlog.Error("Failed to send message to channel",
					xlog.Err(err),
					xlog.Str("channel", channel),
					xlog.Any("kind", uint8(m.Kind)))
			} else {
				xlog.Info("Sent message to channel",
					xlog.Str("channel", channel),
					xlog.I32("total", total),
					xlog.I32("success", success),
					xlog.Any("kind", uint8(m.Kind)))
			}
		}()

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
		headers := []sarama.RecordHeader{{Key: []byte("messageKind"), Value: []byte{byte(m.Kind)}}}
		if toUserID != "" {
			headers = append(headers, sarama.RecordHeader{Key: []byte("toUser"), Value: []byte(toUserID)})
		}
		if toChannel != "" {
			headers = append(headers, sarama.RecordHeader{Key: []byte("toChannel"), Value: []byte(toChannel)})
		}
		if id != nil {
			headers = append(headers,
				sarama.RecordHeader{Key: []byte("fromUser"), Value: []byte(id.UserID)},
				sarama.RecordHeader{Key: []byte("fromClient"), Value: []byte(id.ClientID)})
		}
		kafkaMsg := &sarama.ProducerMessage{
			Topic:   route.KafkaTopic,
			Value:   sarama.ByteEncoder(m.Payload),
			Headers: headers,
		}
		partition, offset, err := b.producer.SendMessage(kafkaMsg)
		if err != nil {
			xlog.Error("Failed to send message to Kafka",
				xlog.Err(err),
				xlog.Str("topic", route.KafkaTopic),
				xlog.Int("kind", int(m.Kind)))
		} else {
			xlog.Debug("Sent message to Kafka",
				xlog.Str("topic", route.KafkaTopic),
				xlog.I32("partition", partition),
				xlog.I64("offset", offset),
				xlog.Int("kind", int(m.Kind)))
		}
	}

	return nil
}

func (b *booter) GetChannels(uid string) (map[string]string, error) {
	if b.redis == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}
	return b.redis.HGetAll(context.Background(), uid).Result()
}
