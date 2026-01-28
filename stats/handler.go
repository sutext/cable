package stats

import (
	"context"
	"time"

	"google.golang.org/grpc/stats"
	"sutext.github.io/cable/packet"
)

type GrpcHandler struct {
	Client stats.Handler
	Server stats.Handler
}

type ConnBegin struct {
	BeginTime time.Time
}

type ConnEnd struct {
	BeginTime time.Time
	EndTime   time.Time
	Code      packet.ConnectCode
	Error     error
}

type MessageBegin struct {
	Kind        packet.MessageKind
	Network     string
	BeginTime   time.Time
	IsIncoming  bool
	PayloadSize int // Size of the payload in bytes
}

type MessageEnd struct {
	Kind       packet.MessageKind
	Error      error
	Network    string
	EndTime    time.Time
	BeginTime  time.Time
	IsIncoming bool
}

type RequestBegin struct {
	Method     string // Method name of the request
	Network    string
	BodySize   int // Size of the request body in bytes
	BeginTime  time.Time
	IsIncoming bool
}

type RequestEnd struct {
	Error      error
	EndTime    time.Time
	Method     string
	Network    string
	BodySize   int
	BeginTime  time.Time
	StatusCode packet.StatusCode
	IsIncoming bool
}

type Handler interface {
	ConnectBegin(ctx context.Context, info *ConnBegin) context.Context
	ConnectEnd(ctx context.Context, info *ConnEnd)
	MessageBegin(ctx context.Context, info *MessageBegin) context.Context
	MessageEnd(ctx context.Context, info *MessageEnd)
	RequestBegin(ctx context.Context, info *RequestBegin) context.Context
	RequestEnd(ctx context.Context, info *RequestEnd)
}
