package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/quic"
	"google.golang.org/grpc/stats"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
	"sutext.github.io/cable/xlog"
)

type RedisClient interface {
	Close() error
	HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
}
type booter struct {
	brokerID            string
	redis               RedisClient
	config              *config
	broker              cluster.Broker
	kafka               sarama.Client
	grpcApi             *grpcServer
	httpApi             *httpServer
	producer            sarama.SyncProducer
	tracerProvider      *tracesdk.TracerProvider
	meterProvider       *metricsdk.MeterProvider
	meter               metric.Meter
	messageUpCounter    metric.Int64Counter
	messageDownCounter  metric.Int64Counter
	messageUpDuration   metric.Float64Histogram
	messageDownDuration metric.Float64Histogram
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
	var serverHandler stats.Handler
	var clientHandler stats.Handler
	if b.config.Trace.Enabled || b.config.Metrics.Enabled {
		serverHandler = otelgrpc.NewServerHandler()
		clientHandler = otelgrpc.NewClientHandler()
	}
	b.broker = cluster.NewBroker(
		cluster.WithBrokerID(b.config.BrokerID),
		cluster.WithHandler(b),
		cluster.WithClusterSize(config.ClusterSize),
		cluster.WithListeners(liss),
		cluster.WithPerrServerHandler(serverHandler),
		cluster.WithPerrClientHandler(clientHandler),
	)

	b.brokerID = fmt.Sprintf("%d", b.broker.ID())
	b.grpcApi = newGRPC(b)
	b.httpApi = newHTTP(b)
	return b
}
func (b *booter) Start() {
	b.startRedis()
	b.startKafka()
	b.startMeter()
	b.startTracer()
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
func (b *booter) startRedis() {
	if b.config.Redis.Enabled {
		b.redis = redis.NewClient(&redis.Options{
			Addr:     b.config.Redis.Address,
			Password: b.config.Redis.Password,
			DB:       b.config.Redis.DB,
		})
		return
	}
	if b.config.RedisCluster.Enabled {
		b.redis = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    b.config.RedisCluster.Addresses,
			Password: b.config.RedisCluster.Password,
		})
	}
}
func (b *booter) startKafka() {
	if !b.config.Kafka.Enabled {
		return
	}
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.Return.Successes = true
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
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
func (b *booter) startTracer() {
	if !b.config.Trace.Enabled {
		return
	}
	ctx := context.Background()
	// Create OTLP exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(b.config.Trace.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // Remove this in production
		otlptracegrpc.WithTimeout(5*time.Second))
	if err != nil {
		xlog.Error("Failed to create otlp tracer exporter", xlog.Err(err))
		return
	}
	// Create resource with service name
	r, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", b.config.Trace.ServiceName),
		attribute.String("service.instance.id", fmt.Sprintf("%d", b.config.BrokerID)),
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
func (b *booter) startMeter() {
	if !b.config.Metrics.Enabled {
		return
	}
	ctx := context.Background()
	// Create OTLP metrics exporter (for Grafana Mimir)
	otlpExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(b.config.Metrics.OTLPEndpoint),
		otlpmetrichttp.WithInsecure(), // Remove this in production
		otlpmetrichttp.WithTimeout(5*time.Second))
	if err != nil {
		xlog.Error("Failed to create otlp metrics exporter", xlog.Err(err))
		return
	}
	// Create meter provider with OTLP exporter for Mimir
	reader := metricsdk.NewPeriodicReader(otlpExporter,
		metricsdk.WithInterval(time.Duration(b.config.Metrics.Interval)*time.Second),
		metricsdk.WithTimeout(5*time.Second),
	)
	r, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", b.config.Metrics.ServiceName),
		attribute.String("service.instance.id", fmt.Sprintf("%d", b.config.BrokerID)),
	))
	if err != nil {
		xlog.Error("Failed to create resource for metrics", xlog.Err(err))
		return
	}
	b.meterProvider = metricsdk.NewMeterProvider(
		metricsdk.WithReader(reader),
		metricsdk.WithResource(r),
	)
	// Set global meter provider
	otel.SetMeterProvider(b.meterProvider)
	// Get meter from provider
	b.meter = b.meterProvider.Meter(
		b.config.Trace.ServiceName,
		metric.WithInstrumentationVersion("1.0.0"),
	)
	// Create metrics instruments
	b.messageUpCounter, err = b.meter.Int64Counter(
		"cable_messages_up_total",
		metric.WithDescription("Total number of incoming messages"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		xlog.Error("Failed to create message up counter", xlog.Err(err))
		return
	}
	b.messageDownCounter, err = b.meter.Int64Counter(
		"cable_messages_down_total",
		metric.WithDescription("Total number of outgoing messages"),
		metric.WithUnit("{message}"),
	)
	if err != nil {
		xlog.Error("Failed to create message down counter", xlog.Err(err))
		return
	}
	b.messageUpDuration, err = b.meter.Float64Histogram(
		"cable_messages_up_duration_seconds",
		metric.WithDescription("Duration of message processing for incoming messages"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.01, 0.1, 1.0, 10.0),
	)
	if err != nil {
		xlog.Error("Failed to create message up duration histogram", xlog.Err(err))
		return
	}
	b.messageDownDuration, err = b.meter.Float64Histogram(
		"cable_messages_down_duration_seconds",
		metric.WithDescription("Duration of message processing for outgoing messages"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.01, 0.1, 1.0, 10.0),
	)
	if err != nil {
		xlog.Error("Failed to create message down duration histogram", xlog.Err(err))
		return
	}
	xlog.Info("OTel metrics initialized for Mimir",
		xlog.Str("otlp_endpoint", b.config.Metrics.OTLPEndpoint),
	)
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
	if b.meterProvider != nil {
		if err := b.meterProvider.Shutdown(ctx); err != nil {
			xlog.Error("Failed to shutdown meter provider", xlog.Err(err))
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
	route, ok := b.config.MessageRoute[m.Kind]
	if !ok || !route.Enabled {
		return nil
	}
	ctx := context.Background()
	kindStr := fmt.Sprintf("%d", m.Kind)
	startTime := time.Now()
	if b.messageUpCounter != nil {
		b.messageUpCounter.Add(context.Background(), 1,
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("kind", kindStr),
			))
	}
	if b.messageUpDuration != nil {
		defer func() {
			elapsed := time.Since(startTime).Seconds()
			b.messageUpDuration.Record(context.Background(), elapsed,
				metric.WithAttributes(
					attribute.String("broker_id", b.brokerID),
					attribute.String("kind", kindStr),
				))
		}()
	}
	if b.tracerProvider != nil {
		tracer := otel.Tracer(b.config.Trace.ServiceName)
		var span trace.Span
		ctx, span = tracer.Start(ctx, "message.process")
		span.SetAttributes(
			attribute.String("message.kind", kindStr),
		)
		defer span.End()
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
			xlog.Str("kind", kindStr))
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
func (b *booter) JoinChannel(ctx context.Context, uid string, channels map[string]string) error {
	if b.redis != nil {
		_, err := b.redis.HSet(ctx, b.userKey(uid), "channels", channels).Result()
		if err != nil {
			return err
		}
	}
	return b.broker.JoinChannel(ctx, uid, channels)

}
func (b *booter) LeaveChannel(ctx context.Context, uid string, channels map[string]string) error {
	if b.redis != nil {
		_, err := b.redis.HSet(ctx, b.userKey(uid), "channels", channels).Result()
		if err != nil {
			return err
		}
	}
	return b.broker.LeaveChannel(ctx, uid, channels)
}

func (b *booter) SendToAll(ctx context.Context, m *packet.Message) (int32, int32, error) {
	kindStr := fmt.Sprintf("%d", m.Kind)
	var resendSpan trace.Span
	if b.tracerProvider != nil {
		tracer := otel.Tracer(b.config.Trace.ServiceName)
		ctx, resendSpan = tracer.Start(ctx, "message.resend.all")
		defer resendSpan.End()
	}
	sendStartTime := time.Now()
	total, success, err := b.broker.SendToAll(ctx, m)
	if b.messageDownCounter != nil {
		b.messageDownCounter.Add(ctx, int64(success),
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("message_kind", kindStr),
			))
	}
	if b.messageDownDuration != nil {
		b.messageDownDuration.Record(ctx, time.Since(sendStartTime).Seconds(),
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("message_kind", kindStr),
			))
	}

	if err != nil {
		xlog.Error("Failed to send message to all", xlog.Err(err))
		if resendSpan != nil {
			resendSpan.RecordError(err)
		}
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
	var resendSpan trace.Span
	if b.tracerProvider != nil {
		tracer := otel.Tracer(b.config.Trace.ServiceName)
		ctx, resendSpan = tracer.Start(ctx, "message.resend.user")
		defer resendSpan.End()
	}
	sendStartTime := time.Now()
	total, success, err := b.broker.SendToUser(ctx, uid, m)
	if b.messageDownCounter != nil {
		b.messageDownCounter.Add(ctx, int64(success),
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("message_kind", kindStr),
			))
	}
	if b.messageDownDuration != nil {
		b.messageDownDuration.Record(ctx, time.Since(sendStartTime).Seconds(),
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("message_kind", kindStr),
			))
	}
	if err != nil {
		xlog.Error("Failed to send message to user",
			xlog.Err(err),
			xlog.Uid(uid),
			xlog.Str("kind", kindStr))
		if resendSpan != nil {
			resendSpan.RecordError(err)
		}
	} else {
		xlog.Info("Sent message to user",
			xlog.Uid(uid),
			xlog.I32("total", total),
			xlog.I32("success", success),
			xlog.Str("kind", kindStr))
		if resendSpan != nil {
			resendSpan.SetAttributes(
				attribute.Int("clients.total", int(total)),
				attribute.Int("clients.success", int(success)),
			)
		}
	}
	return total, success, err
}
func (b *booter) SendToChannel(ctx context.Context, channel string, m *packet.Message) (int32, int32, error) {
	kindStr := fmt.Sprintf("%d", m.Kind)
	var resendSpan trace.Span
	if b.tracerProvider != nil {
		tracer := otel.Tracer(b.config.Trace.ServiceName)
		ctx, resendSpan = tracer.Start(ctx, "message.resend.channel")
		defer resendSpan.End()
	}
	sendStartTime := time.Now()
	total, success, err := b.broker.SendToChannel(ctx, channel, m)
	if b.messageDownCounter != nil {
		b.messageDownCounter.Add(ctx, int64(success),
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("message_kind", kindStr),
			))
	}
	if b.messageDownDuration != nil {
		b.messageDownDuration.Record(ctx, time.Since(sendStartTime).Seconds(),
			metric.WithAttributes(
				attribute.String("broker_id", b.brokerID),
				attribute.String("message_kind", kindStr),
			))
	}
	if err != nil {
		xlog.Error("Failed to send message to channel",
			xlog.Err(err),
			xlog.Str("channel", channel),
			xlog.Str("kind", kindStr))
		if resendSpan != nil {
			resendSpan.RecordError(err)
		}

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
	} else {
		xlog.Debug("Sent message to Kafka",
			xlog.Str("topic", topic),
			xlog.I32("partition", partition),
			xlog.I64("offset", offset),
			xlog.Int("kind", int(m.Kind)))
	}
	if b.tracerProvider != nil {
		tracer := b.tracerProvider.Tracer(b.config.Trace.ServiceName)
		_, kafkaSpan := tracer.Start(ctx, "message.resend.kafka")
		kafkaSpan.SetAttributes(attribute.String("kafka.topic", topic))
		kafkaSpan.SetAttributes(attribute.String("message.kind", fmt.Sprintf("%d", m.Kind)))
		if err != nil {
			kafkaSpan.RecordError(err)
		}
		kafkaSpan.End()
	}
}
