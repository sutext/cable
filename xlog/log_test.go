package xlog

import (
	"fmt"
	"log/slog"
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func BenchmarkBigInfo(b *testing.B) {

	sout, err := os.OpenFile("log.slog.txt", os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	slogger := slog.New(slog.NewJSONHandler(sout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	encoding := zap.NewProductionEncoderConfig()
	encoding.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg := zap.Config{
		Level: zap.NewAtomicLevelAt(zap.InfoLevel),
		// Sampling: &zap.SamplingConfig{
		// 	Initial:    100,
		// 	Thereafter: 100,
		// },
		Encoding:          "json",
		Development:       false,
		EncoderConfig:     encoding,
		DisableCaller:     true,
		DisableStacktrace: true,
		OutputPaths:       []string{"log.zap.txt"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	zlogger, _ := cfg.Build(zap.AddCallerSkip(1))
	b.Run("xlog.info", func(b *testing.B) {
		var i int
		for b.Loop() {
			i++
			slogger.Info("This is a big info message"+fmt.Sprint(i),
				slog.String("key1", "value1"),
				slog.String("key2", "value2"),
				slog.String("key3", "value3"),
				slog.String("key4", "value4"),
				slog.String("key5", "value5"),
				slog.String("key6", "value6"),
				slog.String("key7", "value7"),
				slog.String("key8", "value8"),
				slog.String("key9", "value9"),
				slog.String("key10", "value10"),
				slog.String("key11", "value11"),
				slog.String("key12", "value12"),
				slog.String("key13", "value13"),
				slog.Int("key14", 1),
				slog.Int("key15", 2),
				slog.Int("key16", 3),
			)
		}
	})
	b.Run("zap.info", func(b *testing.B) {
		var i int
		for b.Loop() {
			i++
			zlogger.Info("This is a big info message"+fmt.Sprint(i),
				zap.String("key1", "value1"),
				zap.String("key2", "value2"),
				zap.String("key3", "value3"),
				zap.String("key4", "value4"),
				zap.String("key5", "value5"),
				zap.String("key6", "value6"),
				zap.String("key7", "value7"),
				zap.String("key8", "value8"),
				zap.String("key9", "value9"),
				zap.String("key10", "value10"),
				zap.String("key11", "value11"),
				zap.String("key12", "value12"),
				zap.String("key13", "value13"),
				zap.Int("key14", 1),
				zap.Int("key15", 2),
				zap.Int("key16", 3),
			)
		}
	})
	os.Remove("log.slog.txt")
	os.Remove("log.zap.txt")
}
