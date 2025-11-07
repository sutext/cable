package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/server"
)

func main() {
	ctx := context.Background()
	logger := logger.NewJSON(slog.LevelDebug)

	s := cable.NewServer(":8080",
		// server.WithQUIC(&quic.Config{
		// 	TLSConfig:            &tls.Config{InsecureSkipVerify: true},
		// 	MaxIdleTimeout:       5 * time.Second,
		// 	MaxBidiRemoteStreams: 1,
		// 	MaxUniRemoteStreams:  1,
		// }),
		server.WithLogger(logger),
	)
	logger.Info("entry server start")
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		logger.Info("entry signal received")
		cancel(fmt.Errorf("entry signal received"))
	}()
	go func() {
		if err := s.Serve(); err != nil {
			cancel(err)
		}
	}()
	<-ctx.Done()
	logger.Info("entry server start graceful shutdown")
	done := make(chan struct{})
	go func() {
		s.Shutdown(ctx)
		close(done)
	}()
	timeout := time.NewTimer(time.Second * 15)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		logger.Warn("entry server graceful shutdown timeout")
	case <-done:
		logger.Debug("entry server graceful shutdown")
	}
}
