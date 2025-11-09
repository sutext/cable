package logger

import (
	"context"
	"log/slog"
	"os"
)

type Level = slog.Level

func NewText(level Level) Logger {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(newRequestContextHandler(handler))
}
func NewJSON(level Level) Logger {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(newRequestContextHandler(handler))
}

type requestContextKey struct{}
type requestContextHandler struct {
	handler slog.Handler
}

var (
	KeyRequestID              = requestContextKey{}
	KeyRemoteIP               = requestContextKey{}
	_            slog.Handler = requestContextHandler{}
)

func newRequestContextHandler(handler slog.Handler) slog.Handler {
	return requestContextHandler{
		handler: handler,
	}
}

func (h requestContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h requestContextHandler) Handle(ctx context.Context, record slog.Record) error {
	if v, ok := ctx.Value(KeyRemoteIP).(string); ok {
		record.AddAttrs(slog.String("remote_ip", v))
	}
	if v, ok := ctx.Value(KeyRequestID).(string); ok {
		record.AddAttrs(slog.String("client_remote_ip", v))
	}
	return h.handler.Handle(ctx, record)
}

func (h requestContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestContextHandler{h.handler.WithAttrs(attrs)}
}

func (h requestContextHandler) WithGroup(name string) slog.Handler {
	return h.handler.WithGroup(name)
}
