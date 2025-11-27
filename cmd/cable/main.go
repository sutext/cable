package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable/broker"
	"sutext.github.io/cable/xlog"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			xlog.Error("pprof server start :", err)
		}
	}()
	go func() {
		<-sigs
		cancel(fmt.Errorf("cable signal received"))
	}()
	broker := broker.NewBroker()
	if err := broker.Start(); err != nil {
		xlog.Error("cable server start :", err)
		return
	}
	xlog.Info("cable server started")
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		if err := broker.Shutdown(context.Background()); err != nil {
			xlog.Error("cable server shutdown :", err)
		}
		close(done)
	}()
	timeout := time.NewTimer(time.Second * 15)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		xlog.Warn("cable server graceful shutdown timeout")
	case <-done:
		xlog.Debug("cable server graceful shutdown")
	}
}
