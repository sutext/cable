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
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	resource "go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"sutext.github.io/cable/stats"
	"sutext.github.io/cable/xlog"
)

type milliDuration struct {
	metric.Float64Histogram
}

func newDuration(meter metric.Meter, name string, description string) milliDuration {
	f, err := meter.Float64Histogram(name,
		metric.WithUnit("ms"),
		metric.WithDescription(description),
	)
	if err != nil {
		otel.Handle(err)
		return milliDuration{noop.Float64Histogram{}}
	}
	return milliDuration{f}
}
func (f milliDuration) Record(ctx context.Context, value float64, labels ...attribute.KeyValue) {
	f.Float64Histogram.Record(ctx, value, metric.WithAttributeSet(attribute.NewSet(labels...)))
}

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
	redisDuration   milliDuration
	kafkaDuration   milliDuration
	connectDuration milliDuration
	messageDuration milliDuration
	requestDuration milliDuration
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
		s.kafkaDuration = newDuration(meter, "cable.kafka.duration", " Kafka processing duration in milliseconds")
		s.redisDuration = newDuration(meter, "cable.redis.duration", " Redis processing duration in milliseconds")
		s.connectDuration = newDuration(meter, "cable.connect.duration", " Connect packet processing duration in milliseconds")
		s.messageDuration = newDuration(meter, "cable.message.duration", " Message packet processing duration in milliseconds")
		s.requestDuration = newDuration(meter, "cable.request.duration", " Request packet processing duration in milliseconds")
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
		semconv.ServiceVersion("1.0.0"),
		semconv.ServiceName(conf.ServiceName),
		semconv.ServiceNamespace("cable"),
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
	xlog.Info("OTel tracing initialized", xlog.Str("otle_endpoint", conf.OTLPEndpoint))
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
	xlog.Info("OTel metrics initialized", xlog.Str("otlp_endpoint", conf.OTLPEndpoint))
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

func (s *statistics) MessageBegin(ctx context.Context, info *stats.MessageBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	var inout string
	var kind trace.SpanKind
	if info.IsIncoming {
		inout = "in"
		kind = trace.SpanKindServer
	} else {
		inout = "out"
		kind = trace.SpanKindClient
	}
	ctx, _ = s.tracer.Start(ctx, "cable.message",
		trace.WithSpanKind(kind),
		trace.WithAttributes(
			attribute.Int("cable.message.kind", int(info.Kind)),
			attribute.String("cable.message.network", info.Network),
			attribute.String("cable.message.inout", inout),
		),
	)
	return ctx
}

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
		inout = "in"
	} else {
		inout = "out"
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.messageDuration.Record(ctx, elapsedTime,
		attribute.Int("kind", int(info.Kind)),
		attribute.Bool("isok", info.Error == nil),
		attribute.String("inout", inout),
		attribute.String("network", info.Network),
	)
}

func (s *statistics) RequestBegin(ctx context.Context, info *stats.RequestBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	var inout string
	var kind trace.SpanKind
	if info.IsIncoming {
		inout = "in"
		kind = trace.SpanKindServer
	} else {
		inout = "out"
		kind = trace.SpanKindClient
	}
	ctx, _ = s.tracer.Start(ctx, "cable.request",
		trace.WithSpanKind(kind),
		trace.WithAttributes(
			attribute.String("cable.request.method", info.Method),
			attribute.String("cable.request.network", info.Network),
			attribute.String("cable.request.inout", inout),
		),
	)
	return ctx
}

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
	var inout string
	if info.IsIncoming {
		inout = "in"
	} else {
		inout = "out"
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.requestDuration.Record(ctx, elapsedTime,
		attribute.Bool("isok", info.Error == nil),
		attribute.String("method", info.Method),
		attribute.String("network", info.Network),
		attribute.String("inout", inout),
	)
}

func (s *statistics) RedisBegin(ctx context.Context, info *RedisBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	ctx, _ = s.tracer.Start(ctx, "cable.redis",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("cable.redis.cmd", info.Cmd),
		),
	)
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
	s.redisDuration.Record(ctx, elapsedTime,
		attribute.Bool("isok", info.Error == nil),
		attribute.String("cmd", info.Cmd),
	)
}

func (s *statistics) KafkaBegin(ctx context.Context, info *KafkaBegin) context.Context {
	if s.tracer == nil {
		return ctx
	}
	var op string
	if info.IsProducer {
		op = "produce"
	} else {
		op = "consume"
	}
	ctx, _ = s.tracer.Start(ctx, "cable.kafka",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("cable.kafka.op", op),
			attribute.String("cable.kafka.topic", info.Topic),
		),
	)
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
	s.kafkaDuration.Record(ctx, elapsedTime,
		attribute.String("op", op),
		attribute.String("topic", info.Topic),
		attribute.Bool("isok", info.Error == nil),
	)
}
