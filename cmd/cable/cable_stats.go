package main

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.37.0/messagingconv"
	"go.opentelemetry.io/otel/trace"

	"sutext.github.io/cable/stats"
)

type statistics struct {
	tracer         trace.Tracer
	traceProvider  trace.TracerProvider
	meterProvider  metric.MeterProvider
	connectProcess messagingconv.ProcessDuration
	messageProcess messagingconv.ProcessDuration
	requestProcess messagingconv.ProcessDuration
}

func newStats() stats.Handler {
	s := &statistics{}
	s.meterProvider = otel.GetMeterProvider()
	s.traceProvider = otel.GetTracerProvider()
	s.tracer = s.traceProvider.Tracer("cable.stats", trace.WithInstrumentationVersion("1.0.0"))
	meter := s.meterProvider.Meter("cable.stats", metric.WithInstrumentationVersion("1.0.0"))
	var err error
	s.connectProcess, err = messagingconv.NewProcessDuration(meter)
	if err != nil {
		otel.Handle(err)
	}
	s.messageProcess, err = messagingconv.NewProcessDuration(meter)
	if err != nil {
		otel.Handle(err)
	}
	s.requestProcess, err = messagingconv.NewProcessDuration(meter)
	if err != nil {
		otel.Handle(err)
	}
	return s
}

// TagConn implements stats.Handler.
func (s *statistics) ConnectBegin(ctx context.Context, info *stats.ConnBegin) context.Context {
	ctx, _ = s.tracer.Start(ctx, "cable.user.connect")
	return ctx
}

// HandleConn implements stats.Handler.
func (s *statistics) ConnectEnd(ctx context.Context, info *stats.ConnEnd) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		if info.Error != nil {
			span.RecordError(info.Error)
		}
		span.End()
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.connectProcess.Record(ctx, elapsedTime, "process.connect", "cable")
}

// TagMessage implements stats.Handler.
func (s *statistics) MessageBegin(ctx context.Context, info *stats.MessageBegin) context.Context {
	if info.IsIncoming {
		ctx, _ = s.tracer.Start(ctx, "cable.message.receive",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.Int("cable.message.kind", int(info.Kind)),
				attribute.Int("cable.message.id", int(info.ID)),
				attribute.Int("cable.message.qos", int(info.Qos)),
				attribute.Int("cable.message.payload_size", info.PayloadSize),
			),
		)
	} else {
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.Int("cable.message.kind", int(info.Kind)),
				attribute.Int("cable.message.id", int(info.ID)),
				attribute.Int("cable.message.qos", int(info.Qos)),
				attribute.Int("cable.message.payload_size", info.PayloadSize),
			),
		}
		if s := trace.SpanContextFromContext(ctx); s.IsValid() && s.IsRemote() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: s}))
			ctx = trace.ContextWithRemoteSpanContext(ctx, s)
		}
		ctx, _ = s.tracer.Start(ctx, "cable.message.receive", opts...)
	}

	return ctx
}

// HandleMessage implements stats.Handler.
func (s *statistics) MessageEnd(ctx context.Context, info *stats.MessageEnd) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		if info.Error != nil {
			span.RecordError(info.Error)
		}
		span.End()
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.messageProcess.Record(ctx, elapsedTime, "process.message", "cable")
}

// TagRequest implements stats.Handler.
func (s *statistics) RequestBegin(ctx context.Context, info *stats.RequestBegin) context.Context {
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
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		if info.Error != nil {
			span.RecordError(info.Error)
		}
		span.End()
	}
	elapsedTime := float64(info.EndTime.Sub(info.BeginTime)) / float64(time.Millisecond)
	s.requestProcess.Record(ctx, elapsedTime, "process.request", "cable")
}
