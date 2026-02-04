// // Package xlog provides a unified logging system based on Go's standard library slog.
// // It supports both text and JSON output formats, multiple log levels, and convenient logging functions.
// // The package includes helper functions for common log fields used in the cable project.
package xlog

// import (
// 	"context"
// 	"log/slog"
// 	"os"
// 	"sync/atomic"
// )

// type Attr = slog.Attr

// // Logger wraps a slog.Logger with additional functionality.
// type Logger struct {
// 	s *slog.Logger
// }
// type Level int

// // Log level constants.
// const (
// 	LevelDebug Level = 0  // Debug level logging
// 	LevelInfo  Level = 2  // Info level logging
// 	LevelWarn  Level = 4  // Warning level logging
// 	LevelError Level = 8  // Error level logging
// 	LevelFatal Level = 16 // Fatal level logging
// )

// func (l Level) level() slog.Level {
// 	switch l {
// 	case LevelDebug:
// 		return slog.LevelDebug
// 	case LevelInfo:
// 		return slog.LevelInfo
// 	case LevelWarn:
// 		return slog.LevelWarn
// 	case LevelError:
// 		return slog.LevelError
// 	case LevelFatal:
// 		return slog.Level(LevelFatal)
// 	default:
// 		return slog.LevelInfo
// 	}
// }

// // Re-exported slog attribute constructors for convenience.
// var (
// 	Int  = slog.Int      // Int creates an int attribute
// 	I64  = slog.Int64    // I64 creates an int64 attribute
// 	U64  = slog.Uint64   // U64 creates a uint64 attribute
// 	F64  = slog.Float64  // F64 creates a float64 attribute
// 	Str  = slog.String   // Str creates a string attribute
// 	Dur  = slog.Duration // Dur creates a duration attribute
// 	Any  = slog.Any      // Any creates an attribute for any value
// 	Bool = slog.Bool     // Bool creates a boolean attribute
// 	Time = slog.Time     // Time creates a time attribute
//  Default = NewText(LevelInfo)
// )

// // I16 creates an attribute for an int16 value.
// func I16(key string, i int16) Attr {
// 	return Int(key, int(i))
// }

// // U16 creates an attribute for a uint16 value.
// func U16(key string, i uint16) Attr {
// 	return U64(key, uint64(i))
// }

// // I32 creates an attribute for an int32 value.
// func I32(key string, i int32) Attr {
// 	return Int(key, int(i))
// }

// // U32 creates an attribute for a uint32 value.
// func U32(key string, i uint32) Attr {
// 	return U64(key, uint64(i))
// }

// // F32 creates an attribute for a float32 value.
// func F32(key string, f float32) Attr {
// 	return F64(key, float64(f))
// }

// // Uid creates a user ID attribute with the key "userId".
// func Uid(id string) Attr {
// 	return Str("userId", id)
// }

// // Cid creates a client ID attribute with the key "clientId".
// func Cid(id string) Attr {
// 	return Str("clientId", id)
// }
// func Err(err error) Attr {
// 	return Any("error", err)
// }
// func Ctx(ctx context.Context) Attr {
// 	return Any("context", ctx)
// }
// // Msg creates a message attribute with the key "message".
// func Msg(msg string) Attr {
// 	return Str("message", msg)
// }

// // Peer creates a peer ID attribute with the key "peerId".
// func Peer(id uint64) Attr {
// 	return U64("peerId", id)
// }

// // Channel creates a channel ID attribute with the key "channelId".
// func Channel(ch string) Attr {
// 	return Str("channelId", ch)
// }

// // ParseLevel parses a string level into a slog.Level.
// // Returns LevelInfo for unknown levels.
// func ParseLevel(level string) Level {
// 	switch level {
// 	case "debug":
// 		return LevelDebug
// 	case "info":
// 		return LevelInfo
// 	case "warn":
// 		return LevelWarn
// 	case "error":
// 		return LevelError
// 	case "fatal":
// 		return LevelFatal
// 	default:
// 		return LevelInfo
// 	}
// }
// func NewText(level Level) *Logger {
// 	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
// 		Level: level.level(),
// 	})
// 	return New(slog.New(handler))
// }
// func NewJSON(level Level) *Logger {
// 	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
// 		Level: level.level(),
// 	})
// 	return New(slog.New(handler))
// }
// func New(raw *slog.Logger) *Logger {
// 	return &Logger{s: raw}
// }

// // With creates a new logger with additional attributes added to this logger.
// func (l *Logger) With(attrs ...Attr) *Logger {
// 	le := len(attrs)
// 	if le == 0 {
// 		return l
// 	}
// 	args := make([]any, le)
// 	for i := range le {
// 		args[i] = attrs[i]
// 	}
// 	return &Logger{s: l.s.With(args...)}
// }

// // Debug logs a debug message with optional fields.
// func (l *Logger) Debug(msg string, fields ...Attr) {
// 	l.s.LogAttrs(context.Background(), slog.LevelDebug, msg, fields...)
// }

// // Info logs an info message with optional fields.
// func (l *Logger) Info(msg string, fields ...Attr) {
// 	l.s.LogAttrs(context.Background(), slog.LevelInfo, msg, fields...)
// }

// // Warn logs a warning message with optional fields.
// func (l *Logger) Warn(msg string, fields ...Attr) {
// 	l.s.LogAttrs(context.Background(), slog.LevelWarn, msg, fields...)
// }

// // Error logs an error message with optional fields.
// func (l *Logger) Error(msg string, fields ...Attr) {
// 	l.s.LogAttrs(context.Background(), slog.LevelError, msg, fields...)
// }

// // Fatal logs a fatal message with optional fields.
// func (l *Logger) Fatal(msg string, fields ...Attr) {
// 	l.s.LogAttrs(context.Background(), slog.Level(LevelFatal), msg, fields...)
// }
