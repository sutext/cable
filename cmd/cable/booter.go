package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/net/quic"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type booter struct {
	brokerID            string
	redis               *redis.ClusterClient
	config              *config
	broker              cluster.Broker
	kafka               sarama.Client
	grpcApi             *grpcServer
	httpApi             *httpServer
	producer            sarama.SyncProducer
	tracerProvider      *tracesdk.TracerProvider
	messageUpCounter    *prometheus.CounterVec
	messageDownCounter  *prometheus.CounterVec
	messageUpDuration   *prometheus.HistogramVec
	messageDownDuration *prometheus.HistogramVec
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
	b.broker = cluster.NewBroker(
		cluster.WithBrokerID(b.config.BrokerID),
		cluster.WithHandler(b),
		cluster.WithClusterSize(config.ClusterSize),
		cluster.WithListeners(liss),
	)
	b.brokerID = fmt.Sprintf("%d", b.broker.ID())
	b.grpcApi = newGRPC(b)
	b.httpApi = newHTTP(b)
	return b
}
func (b *booter) Start() {
	// Initialize Redis
	rconfg := b.config.Redis
	b.redis = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    rconfg.Addresses,
		Password: rconfg.Password, // no password set
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

	// Initialize tracing
	if b.config.Trace.Enabled {
		b.initTracing()
	}

	// Initialize metrics and start metrics server
	if b.config.Metrics.Enabled {
		b.initMetrics()
	}
	// Start API server
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

func (b *booter) initTracing() {
	ctx := context.Background()

	// Create OTLP exporter
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(b.config.Trace.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // Remove this in production
		otlptracegrpc.WithTimeout(5*time.Second))
	if err != nil {
		xlog.Error("Failed to create OTLP exporter", xlog.Err(err))
		return
	}

	// Create resource with service name
	r, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", b.config.Trace.ServiceName),
		attribute.String("service.instance.id", fmt.Sprintf("%d", b.config.BrokerID)),
		attribute.String("resource.name", b.config.Trace.ServiceName),
	))
	if err != nil {
		xlog.Error("Failed to create resource", xlog.Err(err))
		return
	}

	// Create tracer provider
	tracerProvider := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(r),
		// Use default sampler for simplicity
	)
	// Apply sampler ratio by setting the global tracer provider's sampler
	// This is a simplified approach to avoid API compatibility issues

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	b.tracerProvider = tracerProvider
	xlog.Info("Tracing initialized", xlog.Str("serviceName", b.config.Trace.ServiceName))
}

func (b *booter) initMetrics() {

	// messageUpCounter counts incoming messages
	b.messageUpCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cable_messages_up_total",
			Help: "Total number of incoming messages",
		},
		[]string{"broker_id", "kind"},
	)

	// messageDownCounter counts outgoing messages
	b.messageDownCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cable_messages_down_total",
			Help: "Total number of outgoing messages",
		},
		[]string{"broker_id", "kind", "target_type"},
	)

	// messageUpRate tracks incoming message rate
	b.messageUpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cable_messages_up_duration_seconds",
			Help:    "Duration of message processing for incoming messages",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"broker_id", "kind"},
	)

	// messageDownRate tracks outgoing message rate
	b.messageDownDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cable_messages_down_duration_seconds",
			Help:    "Duration of message processing for outgoing messages",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"broker_id", "kind", "target_type"},
	)

	prometheus.MustRegister(
		b.messageUpCounter,
		b.messageDownCounter,
		b.messageUpDuration,
		b.messageDownDuration,
	)
	b.httpApi.Handle(b.config.Metrics.Path, promhttp.Handler())
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
	if b.tracerProvider != nil {
		if err := b.tracerProvider.Shutdown(ctx); err != nil {
			xlog.Error("Failed to shutdown tracer provider", xlog.Err(err))
		}
	}
	return b.broker.Shutdown(ctx)
}

// -- Handler methods --
func (b *booter) OnUserConnect(c *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}
func (b *booter) OnUserClosed(id *packet.Identity) {

}
func (b *booter) OnUserMessage(m *packet.Message, id *packet.Identity) error {
	// Record incoming message counter and start timer for rate
	kindStr := fmt.Sprintf("%d", m.Kind)
	b.messageUpCounter.WithLabelValues(b.brokerID, kindStr).Inc()
	startTime := time.Now()
	defer func() {
		b.messageUpDuration.WithLabelValues(b.brokerID, kindStr).Observe(time.Since(startTime).Seconds())
	}()
	tracer := otel.Tracer(b.config.Trace.ServiceName)
	ctx, span := tracer.Start(context.Background(), "message.process")
	span.SetAttributes(
		attribute.Int("message.kind", int(m.Kind)),
		attribute.String("message.id", fmt.Sprintf("%d", m.ID)),
	)
	defer span.End()
	route, exists := b.config.MessageRoute[m.Kind]
	if !exists || !route.Enabled {
		span.SetAttributes(attribute.Bool("route.exists", false))
		return nil
	}
	span.SetAttributes(attribute.Bool("route.exists", true))
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
		span.SetAttributes(
			attribute.String("resend.type", string(route.ResendType)),
			attribute.Bool("resend.valid", false),
		)
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
		return nil, fmt.Errorf("redis client not initialized")
	}
	return b.redis.HGetAll(ctx, b.userKey(uid)).Result()
}
func (b *booter) SendToAll(ctx context.Context, m *packet.Message) (int32, int32, error) {
	tracer := otel.Tracer(b.config.Trace.ServiceName)
	resendCtx, resendSpan := tracer.Start(ctx, "message.resend.all")
	kindStr := fmt.Sprintf("%d", m.Kind)
	targetType := "all"
	sendStartTime := time.Now()
	total, success, err := b.broker.SendToAll(resendCtx, m)
	sendDuration := time.Since(sendStartTime).Seconds()
	for range success {
		b.messageDownCounter.WithLabelValues(b.brokerID, kindStr, targetType).Inc()
	}
	b.messageDownDuration.WithLabelValues(b.brokerID, kindStr, targetType).Observe(sendDuration)

	if err != nil {
		xlog.Error("Failed to send message to all", xlog.Err(err))
		resendSpan.RecordError(err)
	} else {
		xlog.Debug("Sent message to all clients",
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Any("kind", uint8(m.Kind)))
		resendSpan.SetAttributes(
			attribute.Int("clients.total", int(total)),
			attribute.Int("clients.success", int(success)),
		)
	}
	resendSpan.End()
	return total, success, err
}
func (b *booter) SendToUser(ctx context.Context, uid string, m *packet.Message) (int32, int32, error) {
	tracer := otel.Tracer(b.config.Trace.ServiceName)
	resendCtx, resendSpan := tracer.Start(ctx, "message.resend.user")
	resendSpan.SetAttributes(attribute.String("user.id", uid))
	kindStr := fmt.Sprintf("%d", m.Kind)
	targetType := "user"
	sendStartTime := time.Now()
	total, success, err := b.broker.SendToUser(resendCtx, uid, m)
	sendDuration := time.Since(sendStartTime).Seconds()
	for range success {
		b.messageDownCounter.WithLabelValues(b.brokerID, kindStr, targetType).Inc()
	}
	b.messageDownDuration.WithLabelValues(b.brokerID, kindStr, targetType).Observe(sendDuration)
	if err != nil {
		xlog.Error("Failed to send message to user",
			xlog.Err(err),
			xlog.Uid(uid),
			xlog.Any("kind", uint8(m.Kind)))
		resendSpan.RecordError(err)
	} else {
		xlog.Info("Sent message to user",
			xlog.Uid(uid),
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Any("kind", uint8(m.Kind)))
		resendSpan.SetAttributes(
			attribute.Int("clients.total", int(total)),
			attribute.Int("clients.success", int(success)),
		)
	}
	resendSpan.End()
	return total, success, err
}
func (b *booter) SendToChannel(ctx context.Context, channel string, m *packet.Message) (int32, int32, error) {
	tracer := otel.Tracer(b.config.Trace.ServiceName)
	resendCtx, resendSpan := tracer.Start(ctx, "message.resend.channel")
	resendSpan.SetAttributes(attribute.String("channel.name", channel))
	kindStr := fmt.Sprintf("%d", m.Kind)
	targetType := "channel"
	sendStartTime := time.Now()
	total, success, err := b.broker.SendToChannel(resendCtx, channel, m)
	sendDuration := time.Since(sendStartTime).Seconds()
	for range success {
		b.messageDownCounter.WithLabelValues(b.brokerID, kindStr, targetType).Inc()
	}
	b.messageDownDuration.WithLabelValues(b.brokerID, kindStr, targetType).Observe(sendDuration)
	if err != nil {
		xlog.Error("Failed to send message to channel",
			xlog.Err(err),
			xlog.Str("channel", channel),
			xlog.Any("kind", uint8(m.Kind)))
		resendSpan.RecordError(err)
	} else {
		xlog.Info("Sent message to channel",
			xlog.Str("channel", channel),
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Any("kind", uint8(m.Kind)))
		resendSpan.SetAttributes(
			attribute.Int("clients.total", int(total)),
			attribute.Int("clients.success", int(success)),
		)
	}
	resendSpan.End()
	return total, success, err
}
func (b *booter) SendToKafka(ctx context.Context, topic, toUserID, toChannel string, id *packet.Identity, m *packet.Message) {
	tracer := otel.Tracer(b.config.Trace.ServiceName)
	_, kafkaSpan := tracer.Start(ctx, "message.send.kafka")
	kafkaSpan.SetAttributes(attribute.String("kafka.topic", topic))
	headers := []sarama.RecordHeader{
		{Key: []byte("messageQos"), Value: []byte{byte(m.Qos)}},
		{Key: []byte("messageDup"), Value: []byte{bool2byte(m.Dup)}},
		{Key: []byte("messageKind"), Value: []byte{byte(m.Kind)}},
	}
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
		Topic:   topic,
		Value:   sarama.ByteEncoder(m.Payload),
		Headers: headers,
	}
	partition, offset, err := b.producer.SendMessage(kafkaMsg)
	if err != nil {
		xlog.Error("Failed to send message to Kafka",
			xlog.Err(err),
			xlog.Str("topic", topic),
			xlog.Int("kind", int(m.Kind)))
		kafkaSpan.RecordError(err)
	} else {
		xlog.Debug("Sent message to Kafka",
			xlog.Str("topic", topic),
			xlog.I32("partition", partition),
			xlog.I64("offset", offset),
			xlog.Int("kind", int(m.Kind)))
		kafkaSpan.SetAttributes(
			attribute.Int("kafka.partition", int(partition)),
			attribute.Int64("kafka.offset", offset),
		)
	}
	kafkaSpan.End()
}
