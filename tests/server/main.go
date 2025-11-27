package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable/server"
)

func main() {
	ctx := context.Background()
	s := server.New(":8080",
		// server.WithQUIC(&quic.Config{
		// 	TLSConfig:            &tls.Config{InsecureSkipVerify: true},
		// 	MaxIdleTimeout:       5 * time.Second,
		// 	MaxBidiRemoteStreams: 1,
		// 	MaxUniRemoteStreams:  1,
		// }),
		server.WithUDP(),
	)
	slog.Info("cable server start")
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		slog.Info("cable signal received")
		cancel(fmt.Errorf("cable signal received"))
	}()
	go func() {
		if err := s.Serve(); err != nil {
			cancel(err)
		}
	}()
	<-ctx.Done()
	slog.Info("cable server start graceful shutdown")
	done := make(chan struct{})
	go func() {
		s.Shutdown(ctx)
		close(done)
	}()
	timeout := time.NewTimer(time.Second * 15)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		slog.Warn("cable server graceful shutdown timeout")
	case <-done:
		slog.Debug("cable server graceful shutdown")
	}
}
