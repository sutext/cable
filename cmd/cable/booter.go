package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"golang.org/x/net/quic"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type booter struct {
	broker           cluster.Broker
	config           *config
	redis            *redis.Client
	userKafaWriter   *kafka.Writer
	groupKafkaWriter *kafka.Writer
	userKafkaReader  *kafka.Reader
	groupKafkaReader *kafka.Reader
	ctx              context.Context
	cancel           context.CancelFunc
}

const (
	MessageKindUser  packet.MessageKind = 1
	MessageKindGroup packet.MessageKind = 2
)

func newBooter(config *config) *booter {
	b := &booter{
		config: config,
	}
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
	// Initialize context for kafka readers
	b.ctx, b.cancel = context.WithCancel(context.Background())

	// Initialize Redis
	rconfg := b.config.Redis
	b.redis = redis.NewClient(&redis.Options{
		Addr:     rconfg.Address,
		Password: rconfg.Password, // no password set
		DB:       rconfg.DB,       // use default DB
	})

	// Initialize Kafka writers for chat and group messages (upstream)
	kconfg := b.config.Kafka
	b.userKafaWriter = kafka.NewWriter(kafka.WriterConfig{
		Brokers: kconfg.Brokers,
		Topic:   kconfg.UserUpTopic,
	})
	b.groupKafkaWriter = kafka.NewWriter(kafka.WriterConfig{
		Brokers: kconfg.Brokers,
		Topic:   kconfg.GroupUpTopic,
	})

	// Initialize Kafka readers for chat and group messages (downstream)
	b.userKafkaReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers: kconfg.Brokers,
		Topic:   kconfg.UserDownTopic,
		GroupID: "cable-chat-consumer",
	})
	b.groupKafkaReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers: kconfg.Brokers,
		Topic:   kconfg.GroupDownTopic,
		GroupID: "cable-group-consumer",
	})

	// Start kafka reader goroutines
	go b.readKafkaMessages(b.userKafkaReader, MessageKindUser)
	go b.readKafkaMessages(b.groupKafkaReader, MessageKindGroup)

	return b.broker.Start()
}
func (b *booter) Shutdown(ctx context.Context) error {
	// Cancel context to stop kafka readers
	b.cancel()

	// Close kafka writers
	if err := b.userKafaWriter.Close(); err != nil {
		xlog.Error("Failed to close chat kafka writer", xlog.Err(err))
		// Continue with shutdown even if kafka close fails
	}
	if err := b.groupKafkaWriter.Close(); err != nil {
		xlog.Error("Failed to close group kafka writer", xlog.Err(err))
		// Continue with shutdown even if kafka close fails
	}

	// Close kafka readers
	if err := b.userKafkaReader.Close(); err != nil {
		xlog.Error("Failed to close chat kafka reader", xlog.Err(err))
		// Continue with shutdown even if kafka close fails
	}
	if err := b.groupKafkaReader.Close(); err != nil {
		xlog.Error("Failed to close group kafka reader", xlog.Err(err))
		// Continue with shutdown even if kafka close fails
	}

	// Close broker
	return b.broker.Shutdown(ctx)
}
func (b *booter) OnConnect(c *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}
func (b *booter) OnClosed(id *packet.Identity) {

}
func (b *booter) OnMessage(m *packet.Message, id *packet.Identity) error {
	// Process message based on kind
	switch m.Kind {
	case MessageKindUser:
		toUserID, ok := m.Get(packet.PropertyUserID)
		if !ok {
			return fmt.Errorf("no user id in message")
		}
		// Create kafka message
		kafkaMsg := kafka.Message{
			Key:   []byte(toUserID),
			Value: m.Payload,
		}
		// Write chat message to chat topic
		if err := b.userKafaWriter.WriteMessages(context.Background(), kafkaMsg); err != nil {
			xlog.Error("Failed to write chat message to Kafka", xlog.Err(err))
			return err
		}
	case MessageKindGroup:
		channel, ok := m.Get(packet.PropertyChannel)
		if !ok {
			return fmt.Errorf("no channel in message")
		}
		// Create kafka message
		kafkaMsg := kafka.Message{
			Key:   []byte(channel),
			Value: m.Payload,
		}
		// Write group message to group topic
		if err := b.groupKafkaWriter.WriteMessages(context.Background(), kafkaMsg); err != nil {
			xlog.Error("Failed to write group message to Kafka", xlog.Err(err))
			return err
		}
	}
	return nil
}

func (b *booter) GetChannels(uid string) ([]string, error) {
	m, err := b.redis.HGetAll(context.Background(), uid).Result()
	if err != nil {
		return nil, err
	}
	channels := make([]string, 0, len(m))
	for k := range m {
		channels = append(channels, k)
	}
	return channels, nil
}

// readKafkaMessages reads messages from kafka and sends them to clients via broker
func (b *booter) readKafkaMessages(reader *kafka.Reader, kind packet.MessageKind) {
	for {
		// Read message from kafka
		msg, err := reader.ReadMessage(b.ctx)
		if err != nil {
			if err == context.Canceled {
				xlog.Info("Kafka reader stopped gracefully", xlog.Str("kind", "message"), xlog.Int("messageKind", int(kind)))
				return
			}
			xlog.Error("Failed to read message from kafka", xlog.Err(err), xlog.Str("kind", "message"), xlog.Int("messageKind", int(kind)))
			continue
		}

		// Create packet message
		pktMsg := packet.NewMessage(rand.Int64(), msg.Value)
		pktMsg.Kind = kind

		switch kind {
		case MessageKindUser:
			// For chat messages, extract user ID from key and send to user
			userID := string(msg.Key)
			totla, success, err := b.broker.SendToUser(b.ctx, userID, pktMsg)
			if err != nil {
				xlog.Error("Failed to send chat message to user", xlog.Str("userId", userID), xlog.Err(err))
			} else {
				xlog.Info("Success to send chat message to user", xlog.Str("userId", userID), xlog.I32("total", totla), xlog.I32("success", success))
			}
		case MessageKindGroup:
			// For group messages, extract channel from key and send to channel
			channel := string(msg.Key)
			totla, success, err := b.broker.SendToChannel(b.ctx, channel, pktMsg)
			if err != nil {
				xlog.Error("Failed to send group message to channel", xlog.Channel(channel), xlog.Err(err))
			} else {
				xlog.Info("Success to send group message to channel", xlog.Channel(channel), xlog.I32("total", totla), xlog.I32("success", success))
			}
		}
	}
}
