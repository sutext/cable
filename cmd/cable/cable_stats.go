package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.32.0/dbconv"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/semconv/v1.37.0/messagingconv"
	"go.opentelemetry.io/otel/semconv/v1.37.0/rpcconv"
	"go.opentelemetry.io/otel/trace"
	"sutext.github.io/cable/stats"
	"sutext.github.io/cable/xlog"
)

type statistics struct {
	stats.Handler
	kafkaHandler
	redisHandler
	brokerID        uint64
	tracer          trace.Tracer
	traceEnabled    bool
	meterEnabled    bool
	traceProvider   *tracesdk.TracerProvider
	meterProvider   *metricsdk.MeterProvider
	redisDuration   dbconv.ClientOperationDuration
	kafkaDuration   messagingconv.ProcessDuration
	connectDuration ConnectDuraion
	messageDuration MessageDuraion
	requestDuration rpcconv.ServerDuration
}

func newStats(config *config) *statistics {
	s := &statistics{
		brokerID:     config.BrokerID,
		traceEnabled: config.Trace.Enabled,
		meterEnabled: config.Metrics.Enabled,
	}
	if s.traceEnabled {
		provider, err := s.initTrace(config.Trace)
		if err != nil {
			otel.Handle(err)
			xlog.Error("Failed to initialize tracing", xlog.Err(err))
		}
		s.traceProvider = provider
		s.tracer = s.traceProvider.Tracer("cable.stats", trace.WithInstrumentationVersion("1.0.0"))
	}
	if s.meterEnabled {
		provider, err := s.initMeter(config.Metrics)
		if err != nil {
			otel.Handle(err)
			xlog.Error("Failed to initialize metrics", xlog.Err(err))
		}
		s.meterProvider = provider
		meter := s.meterProvider.Meter("cable.stats", metric.WithInstrumentationVersion("1.0.0"))
		s.kafkaDuration, err = messagingconv.NewProcessDuration(meter)
		if err != nil {
			otel.Handle(err)
		}
		s.redisDuration, err = dbconv.NewClientOperationDuration(meter)
		if err != nil {
			otel.Handle(err)
		}
		s.connectDuration, err = NewConnectDuration(meter)
		if err != nil {
			otel.Handle(err)
		}
		s.messageDuration, err = NewMessageDuration(meter)
		if err != nil {
			otel.Handle(err)
		}
		s.requestDuration, err = rpcconv.NewServerDuration(meter)
		if err != nil {
			otel.Handle(err)
		}
	}
	return s
}
func (b *statistics) initTrace(conf traceConfig) (*tracesdk.TracerProvider, error) {
	ctx := context.Background()
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(conf.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // Remove this in production
		otlptracegrpc.WithTimeout(5*time.Second))
	if err != nil {
		return nil, err
	}
	r, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(conf.ServiceName),
		semconv.ServiceInstanceID(fmt.Sprintf("%d", b.brokerID)),
	))
	if err != nil {
		return nil, err
	}
	tracerProvider := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(r),
		// Use default sampler for simplicity
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	xlog.Info("Tracing initialized", xlog.Str("serviceName", conf.ServiceName))
	return tracerProvider, nil
}
func (b *statistics) initMeter(conf metricsConfig) (*metricsdk.MeterProvider, error) {
	ctx := context.Background()
	otlpExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(conf.OTLPEndpoint),
		otlpmetrichttp.WithInsecure(), // Remove this in production
		otlpmetrichttp.WithTimeout(5*time.Second),
		otlpmetrichttp.WithHeaders(map[string]string{
			"X-Scope-OrgID": conf.TenantID,
		}),
	)
	if err != nil {
		return nil, err
	}
	// Create meter provider with OTLP exporter for Mimir
	reader := metricsdk.NewPeriodicReader(otlpExporter,
		metricsdk.WithInterval(time.Duration(conf.Interval)*time.Second),
		metricsdk.WithTimeout(5*time.Second),
	)
	r, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(conf.ServiceName),
		semconv.ServiceInstanceID(fmt.Sprintf("%d", b.brokerID)),
	))
	if err != nil {
		return nil, err
	}
	b.meterProvider = metricsdk.NewMeterProvider(
		metricsdk.WithReader(reader),
		metricsdk.WithResource(r),
	)
	// Set global meter provider
	otel.SetMeterProvider(b.meterProvider)
	xlog.Info("OTel metrics initialized for Mimir", xlog.Str("otlp_endpoint", conf.OTLPEndpoint))
	return b.meterProvider, nil
}
func (s *statistics) Shutdown(ctx context.Context) (err error) {
	if s.traceProvider != nil {
		if err = s.traceProvider.Shutdown(ctx); err != nil {
			xlog.Error("Failed to shutdown tracer provider", xlog.Err(err))
		}
	}
	if s.meterProvider != nil {
		if err = s.meterProvider.Shutdown(ctx); err != nil {
			xlog.Error("Failed to shutdown meter provider", xlog.Err(err))
		}
	}
	return err
}

// TagConn implements stats.Handler.
func (s *statistics) ConnectBegin(ctx context.Context, info *stats.ConnBegin) context.Context {
	if !s.traceEnabled && !s.meterEnabled {
		return ctx
	}
	if s.tracer != nil {
		ctx, _ = s.tracer.Start(ctx, "cable.connect", trace.WithSpanKind(trace.SpanKindServer))
	}
	return ctx
}

// HandleConn implements stats.Handler.
func (s *statistics) ConnectEnd(ctx context.Context, info *stats.ConnEnd) {
	if s.traceEnabled {
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			if info.Error != nil {
				span.RecordError(info.Error)
			}
			span.End()
		}
	}

	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.connectDuration.Record(ctx, elapsedTime, attribute.String("ack_code", info.Code.String()))
}

// TagMessage implements stats.Handler.
func (s *statistics) MessageBegin(ctx context.Context, info *stats.MessageBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	if info.IsIncoming {
		ctx, _ = s.tracer.Start(ctx, "cable.message.receive",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.Int("cable.message.kind", int(info.Kind))),
		)
	} else {
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.Int("cable.message.kind", int(info.Kind))),
		}
		if s := trace.SpanContextFromContext(ctx); s.IsValid() && s.IsRemote() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: s}))
			ctx = trace.ContextWithRemoteSpanContext(ctx, s)
		}
		ctx, _ = s.tracer.Start(ctx, "cable.message.sent", opts...)
	}

	return ctx
}

// HandleMessage implements stats.Handler.
func (s *statistics) MessageEnd(ctx context.Context, info *stats.MessageEnd) {
	if s.traceEnabled {
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			if info.Error != nil {
				span.RecordError(info.Error)
			}
			span.End()
		}
	}
	var inout string
	if info.IsIncoming {
		inout = "receive"
	} else {
		inout = "sent"
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.messageDuration.Record(ctx, elapsedTime,
		attribute.Int("kind", int(info.Kind)),
		attribute.Bool("isok", info.Error == nil),
		attribute.String("inout", inout),
	)
}

// TagRequest implements stats.Handler.
func (s *statistics) RequestBegin(ctx context.Context, info *stats.RequestBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	var name string
	if info.IsIncoming {
		name = "cable.request.receive"
	} else {
		name = "cable.request.send"
	}
	ctx, _ = s.tracer.Start(ctx, name)
	return ctx
}

// HandleRequest implements stats.Handler.
func (s *statistics) RequestEnd(ctx context.Context, info *stats.RequestEnd) {
	if s.traceEnabled {
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			if info.Error != nil {
				span.RecordError(info.Error)
			}
			span.End()
		}
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.requestDuration.Record(ctx, elapsedTime)
}

func (s *statistics) RedisBegin(ctx context.Context, info *RedisBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	ctx, _ = s.tracer.Start(ctx, fmt.Sprintf("cable.redis.%s", info.Type))
	return ctx
}

func (s *statistics) RedisEnd(ctx context.Context, info *RedisEnd) {
	if s.traceEnabled {
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			if info.Error != nil {
				span.RecordError(info.Error)
			}
			span.End()
		}
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.redisDuration.Record(ctx, elapsedTime, "redis",
		s.redisDuration.AttrOperationName(info.Type),
		attribute.Bool("isok", info.Error == nil),
	)
}

func (s *statistics) KafkaBegin(ctx context.Context, info *KafkaBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	if info.IsProducer {
		ctx, _ = s.tracer.Start(ctx, "cable.kafka.producer")
	} else {
		ctx, _ = s.tracer.Start(ctx, "cable.kafka.consumer")
	}
	return ctx
}

func (s *statistics) KafkaEnd(ctx context.Context, info *KafkaEnd) {
	if s.traceEnabled {
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			if info.Error != nil {
				span.RecordError(info.Error)
			}
			span.End()
		}
	}
	var op string
	if info.IsProducer {
		op = "produce"
	} else {
		op = "consume"
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.kafkaDuration.Record(ctx, elapsedTime, op, "kafka",
		attribute.String("topic", info.Topic),
		attribute.Bool("isok", info.Error == nil),
	)
}
