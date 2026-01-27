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
	UserID    string // User ID associated with the client
	ClientIP  string // IP address of the client
	ClientID  string // Client ID of the connection
	BeginTime time.Time
}

type ConnEnd struct {
	BeginTime time.Time
	EndTime   time.Time
	Code      packet.ConnectCode
	Error     error
}

type MessageBegin struct {
	ID          uint16 // Message ID of the message
	Qos         packet.MessageQos
	Kind        packet.MessageKind
	BeginTime   time.Time
	IsIncoming  bool
	PayloadSize int // Size of the payload in bytes
}

type MessageEnd struct {
	Kind       packet.MessageKind
	Error      error
	EndTime    time.Time
	BeginTime  time.Time
	IsIncoming bool
}

type RequestBegin struct {
	ID         uint16 // Request ID of the request
	Method     string // Method name of the request
	BodySize   int    // Size of the request body in bytes
	BeginTime  time.Time
	IsIncoming bool
}

type RequestEnd struct {
	Error      error
	EndTime    time.Time
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
