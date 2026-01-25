package stats

import (
	"context"
)

type ConnInfo struct {
	IP  string // IP address of the client
	UID string // User ID associated with the client
	CID string // Client ID of the connection
}
type ConnStats interface {
	isConnStats() // ConnStats is a marker interface for connection statistics.
}
type ConnBegin struct{}

func (*ConnBegin) isConnStats() {}

type ConnEnd struct{}

func (*ConnEnd) isConnStats() {}

type MessageInfo struct {
	MsgID uint16 // Message ID of the message
}
type MessageStats interface {
	isMessageStats() // MessageStats is a marker interface for message statistics.
}
type MessageBegin struct{}

func (*MessageBegin) isMessageStats() {}

type MessageEnd struct{}

func (*MessageEnd) isMessageStats() {}

type RequestInfo struct {
	ReqID uint16 // Request ID of the request
}
type RequestStats interface {
	isRequestStats()
}
type RequestBegin struct{}

func (*RequestBegin) isRequestStats() {}

type RequestEnd struct{}

func (*RequestEnd) isRequestStats() {}

type Handler interface {
	TagConn(ctx context.Context, info *ConnInfo) context.Context
	HandleConn(ctx context.Context, stats ConnStats)
	TagMessage(ctx context.Context, msg *MessageInfo) context.Context
	HandleMessage(ctx context.Context, stats MessageStats)
	TagRequest(ctx context.Context, req *RequestInfo) context.Context
	HandleRequest(ctx context.Context, stats RequestStats)
}
