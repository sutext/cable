package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
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

// Message metrics definitions
var (
	// messageUpCounter counts incoming messages
	messageUpCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cable_messages_up_total",
			Help: "Total number of incoming messages",
		},
		[]string{"broker_id", "kind"},
	)

	// messageDownCounter counts outgoing messages
	messageDownCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cable_messages_down_total",
			Help: "Total number of outgoing messages",
		},
		[]string{"broker_id", "kind", "target_type"},
	)

	// messageUpRate tracks incoming message rate
	messageUpRate = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cable_messages_up_duration_seconds",
			Help:    "Duration of message processing for incoming messages",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"broker_id", "kind"},
	)

	// messageDownRate tracks outgoing message rate
	messageDownRate = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cable_messages_down_duration_seconds",
			Help:    "Duration of message processing for outgoing messages",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"broker_id", "kind", "target_type"},
	)
)

type booter struct {
	redis          *redis.Client
	config         *config
	broker         cluster.Broker
	kafka          sarama.Client
	grpcApi        *grpcServer
	httpApi        *httpServer
	producer       sarama.SyncProducer
	tracerProvider *tracesdk.TracerProvider
	brokerIDStr    string
}

func newBooter(config *config) *booter {
	b := &booter{
		config:      config,
		brokerIDStr: fmt.Sprintf("%d", config.BrokerID),
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
	b.grpcApi = newGRPC(b)
	b.httpApi = newHTTP(b)
	return b
}
func (b *booter) Start() {
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
	// Register metrics with default registry
	prometheus.MustRegister(
		messageUpCounter,
		messageDownCounter,
		messageUpRate,
		messageDownRate,
	)

	// Start metrics HTTP server
	metricsAddr := fmt.Sprintf(":%d", b.config.Metrics.Port)
	go func() {
		b.httpApi.Handle("/metrics", promhttp.Handler())
		xlog.Info("Metrics server started", xlog.Str("address", metricsAddr))
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			xlog.Error("Failed to start metrics server", xlog.Err(err))
		}
	}()
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
func (b *booter) ListChannels(ctx context.Context, uid string) (map[string]string, error) {
	if b.redis == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}
	return b.redis.HGetAll(ctx, b.userKey(uid)).Result()
}
func (b *booter) userKey(uid string) string {
	return fmt.Sprintf("%s:%s", b.config.Redis.UserPrefix, uid)
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
	messageUpCounter.WithLabelValues(b.brokerIDStr, kindStr).Inc()
	startTime := time.Now()
	defer func() {
		messageUpRate.WithLabelValues(b.brokerIDStr, kindStr).Observe(time.Since(startTime).Seconds())
	}()

	// Get tracer
	tracer := otel.Tracer(b.config.Trace.ServiceName)

	// Create root span for message processing
	ctx, span := tracer.Start(context.Background(), "message.process")
	span.SetAttributes(
		attribute.Int("message.kind", int(m.Kind)),
		attribute.String("message.id", fmt.Sprintf("%d", m.ID)),
	)
	defer span.End()

	// Get route configuration for this message kind
	route, exists := b.config.MessageRoute[m.Kind]
	if !exists || !route.Enabled {
		// No route configured, do nothing
		span.SetAttributes(attribute.Bool("route.exists", false))
		return nil
	}
	span.SetAttributes(attribute.Bool("route.exists", true))

	toUserID := ""
	toChannel := ""
	// Handle message resend based on resendType
	switch route.ResendType {
	case resendToAll:
		// Send message to all clients asynchronously
		go func() {
			resendCtx, resendSpan := tracer.Start(ctx, "message.resend.all")

			// Record outgoing message metrics
			kindStr := fmt.Sprintf("%d", m.Kind)
			targetType := "all"
			sendStartTime := time.Now()

			total, success, err := b.broker.SendToAll(resendCtx, m)
			sendDuration := time.Since(sendStartTime).Seconds()

			// Record metrics for each sent message
			for i := int32(0); i < success; i++ {
				messageDownCounter.WithLabelValues(b.brokerIDStr, kindStr, targetType).Inc()
			}
			messageDownRate.WithLabelValues(b.brokerIDStr, kindStr, targetType).Observe(sendDuration)

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
			resendCtx, resendSpan := tracer.Start(ctx, "message.resend.user")
			resendSpan.SetAttributes(attribute.String("user.id", uid))

			// Record outgoing message metrics
			kindStr := fmt.Sprintf("%d", m.Kind)
			targetType := "user"
			sendStartTime := time.Now()

			total, success, err := b.broker.SendToUser(resendCtx, uid, m)
			sendDuration := time.Since(sendStartTime).Seconds()

			// Record metrics for each sent message
			for i := int32(0); i < success; i++ {
				messageDownCounter.WithLabelValues(b.brokerIDStr, kindStr, targetType).Inc()
			}
			messageDownRate.WithLabelValues(b.brokerIDStr, kindStr, targetType).Observe(sendDuration)

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
		}()
	case resendToChannel:
		channel, ok := m.Get(packet.PropertyChannel)
		if !ok {
			return nil
		}
		toChannel = channel
		// Send message to specified channel asynchronously
		go func() {
			resendCtx, resendSpan := tracer.Start(ctx, "message.resend.channel")
			resendSpan.SetAttributes(attribute.String("channel.name", channel))

			// Record outgoing message metrics
			kindStr := fmt.Sprintf("%d", m.Kind)
			targetType := "channel"
			sendStartTime := time.Now()

			total, success, err := b.broker.SendToChannel(resendCtx, channel, m)
			sendDuration := time.Since(sendStartTime).Seconds()

			// Record metrics for each sent message
			for range success {
				messageDownCounter.WithLabelValues(b.brokerIDStr, kindStr, targetType).Inc()
			}
			messageDownRate.WithLabelValues(b.brokerIDStr, kindStr, targetType).Observe(sendDuration)

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
		}()

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
		_, kafkaSpan := tracer.Start(ctx, "message.send.kafka")
		kafkaSpan.SetAttributes(attribute.String("kafka.topic", route.KafkaTopic))

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
			kafkaSpan.RecordError(err)
		} else {
			xlog.Debug("Sent message to Kafka",
				xlog.Str("topic", route.KafkaTopic),
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

	return nil
}
func (b *booter) GetUserChannels(uid string) (map[string]string, error) {
	return b.ListChannels(context.Background(), uid)
}
