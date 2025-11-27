package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable/broker"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			slog.Error("pprof server start :", "error", err)
		}
	}()
	go func() {
		<-sigs
		cancel(fmt.Errorf("cable signal received"))
	}()
	broker := broker.NewBroker()
	if err := broker.Start(); err != nil {
		slog.Error("cable server start :", "error", err)
		return
	}
	slog.Info("cable server started")
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		if err := broker.Shutdown(context.Background()); err != nil {
			slog.Error("cable server shutdown :", "error", err)
		}
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
