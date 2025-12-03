package xlog

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

var defaultLogger atomic.Pointer[Logger]

func init() {
	defaultLogger.Store(NewText(LevelInfo))
}

func Debug(msg string, fields ...slog.Attr) {
	Defualt().Debug(msg, fields...)
}

func Info(msg string, fields ...slog.Attr) {
	Defualt().Info(msg, fields...)
}

func Warn(msg string, fields ...slog.Attr) {
	Defualt().Warn(msg, fields...)
}
func Error(msg string, fields ...slog.Attr) {
	Defualt().Error(msg, fields...)
}

type Logger struct {
	json bool
	s    *slog.Logger
}

const (
	LevelDebug slog.Level = slog.LevelDebug
	LevelInfo  slog.Level = slog.LevelInfo
	LevelWarn  slog.Level = slog.LevelWarn
	LevelError slog.Level = slog.LevelError
)

var (
	Int      = slog.Int
	Any      = slog.Any
	Bool     = slog.Bool
	Time     = slog.Time
	Int64    = slog.Int64
	Uint64   = slog.Uint64
	String   = slog.String
	Float64  = slog.Float64
	Duration = slog.Duration
)

func Err(e error) slog.Attr {
	return slog.Any("error", e)
}
func Uid(user string) slog.Attr {
	return slog.String("userId", user)
}
func Cid(user string) slog.Attr {
	return slog.String("clientId", user)
}
func Msg(msg string) slog.Attr {
	return slog.String("message", msg)
}
func Channel(ch string) slog.Attr {
	return slog.String("channelId", ch)
}
func With(args ...any) *Logger {
	return Defualt().With(args...)
}
func WithLevel(level slog.Level) *Logger {
	return Defualt().WithLevel(level)
}
func NewText(level slog.Level) *Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{s: slog.New(handler), json: false}
}
func NewJSON(level slog.Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{s: slog.New(handler), json: true}
}

func Defualt() *Logger {
	return defaultLogger.Load()
}
func SetDefault(l *Logger) {
	defaultLogger.Store(l)
}
func (l *Logger) With(args ...any) *Logger {
	return &Logger{s: l.s.With(args...)}
}
func (l *Logger) WithLevel(level slog.Level) *Logger {
	if l.json {
		return NewJSON(level)
	}
	return NewText(level)
}
func (l *Logger) Debug(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelDebug, msg, fields...)
}

func (l *Logger) Info(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelInfo, msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelWarn, msg, fields...)
}
func (l *Logger) Error(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelError, msg, fields...)
}
