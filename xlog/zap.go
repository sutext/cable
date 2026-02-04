// Package xlog provides a unified logging system based on Go's standard library slog.
// It supports both text and JSON output formats, multiple log levels, and convenient logging functions.
// The package includes helper functions for common log fields used in the cable project.
package xlog

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Attr = zap.Field

// Logger wraps a slog.Logger with additional functionality.
type Logger struct {
	s *zap.Logger
}
type Level int

// Log level constants.
const (
	LevelDebug Level = 0  // Debug level logging
	LevelInfo  Level = 2  // Info level logging
	LevelWarn  Level = 4  // Warning level logging
	LevelError Level = 8  // Error level logging
	LevelFatal Level = 16 // Fatal level logging
)

func (l Level) Level() zapcore.Level {
	switch l {
	case LevelDebug:
		return zapcore.DebugLevel
	case LevelInfo:
		return zapcore.InfoLevel
	case LevelWarn:
		return zapcore.WarnLevel
	case LevelError:
		return zapcore.ErrorLevel
	case LevelFatal:
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Re-exported zap attribute constructors for convenience.
var (
	Int     = zap.Int      // Int creates an int attribute
	I64     = zap.Int64    // I64 creates an int64 attribute
	U64     = zap.Uint64   // U64 creates a uint64 attribute
	F64     = zap.Float64  // F64 creates a float64 attribute
	Str     = zap.String   // Str creates a string attribute
	Dur     = zap.Duration // Dur creates a duration attribute
	Err     = zap.Error    // Err creates an error attribute
	Any     = zap.Any      // Any creates an attribute for any value
	Bool    = zap.Bool     // Bool creates a boolean attribute
	Time    = zap.Time     // Time creates a time attribute
	Default = NewText(LevelInfo)
)

// I16 creates an attribute for an int16 value.
func I16(key string, i int16) Attr {
	return Int(key, int(i))
}

// U16 creates an attribute for a uint16 value.
func U16(key string, i uint16) Attr {
	return U64(key, uint64(i))
}

// I32 creates an attribute for an int32 value.
func I32(key string, i int32) Attr {
	return Int(key, int(i))
}

// U32 creates an attribute for a uint32 value.
func U32(key string, i uint32) Attr {
	return U64(key, uint64(i))
}

// F32 creates an attribute for a float32 value.
func F32(key string, f float32) Attr {
	return F64(key, float64(f))
}

// Uid creates a user ID attribute with the key "userId".
func Uid(id string) Attr {
	return Str("userId", id)
}

// Cid creates a client ID attribute with the key "clientId".
func Cid(id string) Attr {
	return Str("clientId", id)
}

func Ctx(ctx context.Context) Attr {
	return Any("context", ctx)
}

// Msg creates a message attribute with the key "message".
func Msg(msg string) Attr {
	return Str("message", msg)
}

// Peer creates a peer ID attribute with the key "peerId".
func Peer(id uint64) Attr {
	return U64("peerId", id)
}

// Channel creates a channel ID attribute with the key "channelId".
func Channel(ch string) Attr {
	return Str("channelId", ch)
}

// ParseLevel parses a string level into a slog.Level.
// Returns LevelInfo for unknown levels.
func ParseLevel(level string) Level {
	switch level {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelInfo
	}
}
func NewText(level Level, opts ...zap.Option) *Logger {
	encoding := zap.NewProductionEncoderConfig()
	encoding.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg := zap.Config{
		Level: zap.NewAtomicLevelAt(level.Level()),
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:          "console",
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		EncoderConfig:     encoding,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	opts = append(opts, zap.AddCallerSkip(1))
	s, err := cfg.Build(opts...)
	if err != nil {
		panic(err)
	}
	return New(s)
}
func NewJSON(level Level, opts ...zap.Option) *Logger {
	encoding := zap.NewProductionEncoderConfig()
	encoding.EncodeTime = zapcore.EpochTimeEncoder
	cfg := zap.Config{
		Level: zap.NewAtomicLevelAt(level.Level()),
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:          "json",
		Development:       false,
		EncoderConfig:     encoding,
		DisableCaller:     false,
		DisableStacktrace: true,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	opts = append(opts, zap.AddCallerSkip(1))
	s, err := cfg.Build(opts...)
	if err != nil {
		panic(err)
	}
	return New(s)
}
func New(raw *zap.Logger) *Logger {
	return &Logger{s: raw}
}

// With creates a new logger with additional attributes added to this logger.
func (l *Logger) With(args ...Attr) *Logger {
	return &Logger{s: l.s.With(args...)}
}

// Debug logs a debug message with optional fields.
// Note: xlog.Ctx(ctx) can be used to add a context to the log.
// Waring: xlog.Ctx(ctx) should be the first field in the fields list.
// Example:
//
//	l.Debug("message", xlog.Ctx(ctx),xlog.Str("key", "value"))
func (l *Logger) Debug(msg string, fields ...Attr) {
	l.s.Debug(msg, fields...)
}

// Info logs an info message with optional fields.
// Note: xlog.Ctx(ctx) can be used to add a context to the log.
// Waring: xlog.Ctx(ctx) should be the first field in the fields list.
// Example:
//
//	l.Info("message", xlog.Ctx(ctx),xlog.Str("key", "value"))
func (l *Logger) Info(msg string, fields ...Attr) {
	l.s.Info(msg, fields...)
}

// Warn logs a warning message with optional fields.
// Note: xlog.Ctx(ctx) can be used to add a context to the log.
// Waring: xlog.Ctx(ctx) should be the first field in the fields list.
// Example:
//
//	l.Warn("message", xlog.Ctx(ctx),xlog.Str("key", "value"))
func (l *Logger) Warn(msg string, fields ...Attr) {
	l.s.Warn(msg, fields...)
}

// Error logs an error message with optional fields.
// Note: xlog.Ctx(ctx) can be used to add a context to the log.
// Waring: xlog.Ctx(ctx) should be the first field in the fields list.
// Example:
//
//	l.Error("message", xlog.Ctx(ctx),xlog.Str("key", "value"))
func (l *Logger) Error(msg string, fields ...Attr) {
	l.s.Error(msg, fields...)
}

// Fatal logs a fatal message with optional fields.
// Note: xlog.Ctx(ctx) can be used to add a context to the log.
// Waring: xlog.Ctx(ctx) should be the first field in the fields list.
// Example:
//
//	l.Fatal("message", xlog.Ctx(ctx),xlog.Str("key", "value"))
func (l *Logger) Fatal(msg string, fields ...Attr) {
	l.s.Fatal(msg, fields...)
}
