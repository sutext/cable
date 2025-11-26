package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable/broker"
	"sutext.github.io/cable/internal/logger"
)

func main() {
	ctx := context.Background()
	logger := logger.NewText(slog.LevelDebug)
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	go func() {
		<-sigs
		cancel(fmt.Errorf("cable signal received"))
	}()
	broker := broker.NewBroker()
	if err := broker.Start(); err != nil {
		logger.Error("cable server start error: %v", err)
		return
	}
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		broker.Shutdown(context.Background())
		close(done)
	}()
	timeout := time.NewTimer(time.Second * 15)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		logger.Warn("cable server graceful shutdown timeout")
	case <-done:
		logger.Debug("cable server graceful shutdown")
	}
}
