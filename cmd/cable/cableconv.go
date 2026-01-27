package main

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

type ConnectDuraion struct {
	metric.Float64Histogram
}

var newConnectDurationOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Duration of processing connect request."),
	metric.WithUnit("ms"),
}

func NewConnectDuration(m metric.Meter, opt ...metric.Float64HistogramOption) (ConnectDuraion, error) {
	// Check if the meter is nil.
	if m == nil {
		return ConnectDuraion{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newConnectDurationOpts
	} else {
		opt = append(opt, newConnectDurationOpts...)
	}

	i, err := m.Float64Histogram(
		"cable.connect.duration",
		opt...,
	)
	if err != nil {
		return ConnectDuraion{noop.Float64Histogram{}}, err
	}
	return ConnectDuraion{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (ConnectDuraion) Name() string {
	return "cable.connect.duration"
}

// Unit returns the semantic convention unit of the instrument
func (ConnectDuraion) Unit() string {
	return "ms"
}

// Description returns the semantic convention description of the instrument
func (ConnectDuraion) Description() string {
	return "Duration of processing connect request."
}
func (m ConnectDuraion) Record(ctx context.Context, val float64, attrs ...attribute.KeyValue) {
	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(attrs...))
}

type MessageDuraion struct {
	metric.Float64Histogram
}

var newMessageDurationOpts = []metric.Float64HistogramOption{
	metric.WithDescription("Duration of processing cable message"),
	metric.WithUnit("ms"),
}

func NewMessageDuration(m metric.Meter, opt ...metric.Float64HistogramOption) (MessageDuraion, error) {
	// Check if the meter is nil.
	if m == nil {
		return MessageDuraion{noop.Float64Histogram{}}, nil
	}

	if len(opt) == 0 {
		opt = newMessageDurationOpts
	} else {
		opt = append(opt, newMessageDurationOpts...)
	}

	i, err := m.Float64Histogram(
		"cable.message.duration",
		opt...,
	)
	if err != nil {
		return MessageDuraion{noop.Float64Histogram{}}, err
	}
	return MessageDuraion{i}, nil
}

// Name returns the semantic convention name of the instrument.
func (MessageDuraion) Name() string {
	return "cable.message.duration"
}

// Unit returns the semantic convention unit of the instrument
func (MessageDuraion) Unit() string {
	return "ms"
}

// Description returns the semantic convention description of the instrument
func (MessageDuraion) Description() string {
	return "Duration of processing cable message"
}
func (m MessageDuraion) Record(ctx context.Context, val float64, attrs ...attribute.KeyValue) {
	m.Float64Histogram.Record(ctx, val, metric.WithAttributes(attrs...))
}
