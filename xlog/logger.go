// Package xlog provides a unified logging system based on Go's standard library slog.
// It supports both text and JSON output formats, multiple log levels, and convenient logging functions.
// The package includes helper functions for common log fields used in the cable project.
package xlog

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

// defaultLogger is the default Logger instance used by the package-level logging functions.
var defaultLogger atomic.Pointer[Logger]

func init() {
	defaultLogger.Store(NewText(LevelInfo))
}

// Debug logs a debug message with optional fields.
func Debug(msg string, fields ...slog.Attr) {
	Defualt().Debug(msg, fields...)
}

// Info logs an info message with optional fields.
func Info(msg string, fields ...slog.Attr) {
	Defualt().Info(msg, fields...)
}

// Warn logs a warning message with optional fields.
func Warn(msg string, fields ...slog.Attr) {
	Defualt().Warn(msg, fields...)
}

// Error logs an error message with optional fields.
func Error(msg string, fields ...slog.Attr) {
	Defualt().Error(msg, fields...)
}

// Logger wraps a slog.Logger with additional functionality.
type Logger struct {
	s *slog.Logger // Underlying slog.Logger instance
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

func (l Level) slevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	case LevelFatal:
		return slog.Level(LevelFatal)
	default:
		return slog.LevelInfo
	}
}

// Re-exported slog attribute constructors for convenience.
var (
	Int  = slog.Int      // Int creates an int attribute
	I64  = slog.Int64    // I64 creates an int64 attribute
	U64  = slog.Uint64   // U64 creates a uint64 attribute
	F64  = slog.Float64  // F64 creates a float64 attribute
	Str  = slog.String   // Str creates a string attribute
	Dur  = slog.Duration // Dur creates a duration attribute
	Any  = slog.Any      // Any creates an attribute for any value
	Bool = slog.Bool     // Bool creates a boolean attribute
	Time = slog.Time     // Time creates a time attribute
)

// I16 creates an attribute for an int16 value.
func I16(key string, i int16) slog.Attr {
	return slog.Int(key, int(i))
}

// U16 creates an attribute for a uint16 value.
func U16(key string, i uint16) slog.Attr {
	return slog.Uint64(key, uint64(i))
}

// I32 creates an attribute for an int32 value.
func I32(key string, i int32) slog.Attr {
	return slog.Int(key, int(i))
}

// U32 creates an attribute for a uint32 value.
func U32(key string, i uint32) slog.Attr {
	return slog.Uint64(key, uint64(i))
}

// F32 creates an attribute for a float32 value.
func F32(key string, f float32) slog.Attr {
	return slog.Float64(key, float64(f))
}

// Err creates an error attribute with the key "error".
func Err(e error) slog.Attr {
	return slog.Any("error", e)
}

// Uid creates a user ID attribute with the key "userId".
func Uid(id string) slog.Attr {
	return slog.String("userId", id)
}

// Cid creates a client ID attribute with the key "clientId".
func Cid(id string) slog.Attr {
	return slog.String("clientId", id)
}

// Msg creates a message attribute with the key "message".
func Msg(msg string) slog.Attr {
	return slog.String("message", msg)
}

// Peer creates a peer ID attribute with the key "peerId".
func Peer(id uint64) slog.Attr {
	return slog.Uint64("peerId", id)
}

// Channel creates a channel ID attribute with the key "channelId".
func Channel(ch string) slog.Attr {
	return slog.String("channelId", ch)
}

// With creates a new logger with additional attributes added to the default logger.
func With(args ...any) *Logger {
	return Defualt().With(args...)
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

// NewText creates a new logger with text output format.
func NewText(level Level) *Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level.slevel(),
	})
	return &Logger{s: slog.New(handler)}
}

// NewJSON creates a new logger with JSON output format.
func NewJSON(level Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level.slevel(),
	})
	return &Logger{s: slog.New(handler)}
}
func New(raw *slog.Logger) *Logger {
	return &Logger{s: raw}
}

// Defualt returns the default logger instance.
// Note: There's a typo in the function name, it should be "Default".
func Defualt() *Logger {
	return defaultLogger.Load()
}

// SetDefault sets the default logger instance.
func SetDefault(l *Logger) {
	defaultLogger.Store(l)
}

// With creates a new logger with additional attributes added to this logger.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{s: l.s.With(args...)}
}

// Debug logs a debug message with optional fields.
func (l *Logger) Debug(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelDebug, msg, fields...)
}

// Info logs an info message with optional fields.
func (l *Logger) Info(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelInfo, msg, fields...)
}

// Warn logs a warning message with optional fields.
func (l *Logger) Warn(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelWarn, msg, fields...)
}

// Error logs an error message with optional fields.
func (l *Logger) Error(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.LevelError, msg, fields...)
}
func (l *Logger) Fatal(msg string, fields ...slog.Attr) {
	l.s.LogAttrs(context.Background(), slog.Level(LevelFatal), msg, fields...)
}
