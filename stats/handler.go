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
	Inout       string // "in" or "out"
	Network     string
	BeginTime   time.Time
	PayloadSize int // Size of the payload in bytes
}

type MessageEnd struct {
	Kind      packet.MessageKind
	Error     error
	Inout     string // "in" or "out"
	Network   string
	EndTime   time.Time
	BeginTime time.Time
}

type RequestBegin struct {
	Inout     string // "in" or "out"
	Method    string // Method name of the request
	Network   string
	BodySize  int // Size of the request body in bytes
	BeginTime time.Time
}

type RequestEnd struct {
	Inout      string // "in" or "out"
	Error      error
	EndTime    time.Time
	Method     string
	Network    string
	BodySize   int
	BeginTime  time.Time
	StatusCode packet.StatusCode
}

type Handler interface {
	ConnnectHadler
	MessageHandler
	RequestHandler
	QueueHandler
}
type ConnnectHadler interface {
	ConnectBegin(ctx context.Context, info *ConnBegin) context.Context
	ConnectEnd(ctx context.Context, info *ConnEnd)
	Disconnect(ctx context.Context）
}
type MessageHandler interface {
	MessageBegin(ctx context.Context, info *MessageBegin) context.Context
	MessageEnd(ctx context.Context, info *MessageEnd)
}
type RequestHandler interface {
	RequestBegin(ctx context.Context, info *RequestBegin) context.Context
	RequestEnd(ctx context.Context, info *RequestEnd)
}
type QueueHandler interface {
	UpdateQueue(ctx context.Context, qsize int32)
}
