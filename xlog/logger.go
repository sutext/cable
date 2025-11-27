package xlog

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

var defaultLogger atomic.Pointer[Logger]

func init() {
	defaultLogger.Store(NewText(LevelDebug))
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

func Error(msg string, err error) {
	Defualt().Error(msg, err)
}
func Errorf(msg string, fields ...slog.Attr) {
	Defualt().Errorf(msg, fields...)
}

type Logger struct {
	s *slog.Logger
}

const (
	LevelDebug slog.Level = slog.LevelDebug
	LevelInfo  slog.Level = slog.LevelInfo
	LevelWarn  slog.Level = slog.LevelWarn
	LevelError slog.Level = slog.LevelError
)

func With(args ...any) *Logger {
	return Defualt().With(args...)
}
func NewText(level slog.Level) *Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{s: slog.New(handler)}
}
func NewJSON(level slog.Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{s: slog.New(handler)}
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
func (l *Logger) Debug(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelDebug, msg, fields...)
}

func (l *Logger) Info(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelInfo, msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelWarn, msg, fields...)
}

func (l *Logger) Error(msg string, err error) {
	l.s.LogAttrs(context.Background(), slog.LevelError, msg, slog.Any("error", err))
}
func (l *Logger) Errorf(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelError, msg, fields...)
}
