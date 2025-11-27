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
	"sutext.github.io/cable/server"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			slog.Error("pprof server error:", "error", err)
		}
	}()
	go func() {
		<-sigs
		cancel(fmt.Errorf("entry signal received"))
	}()
	peers := []string{
		"broker1@127.0.0.1:4369",
		"broker2@127.0.0.1:4370",
		"broker3@127.0.0.1:4371",
		"broker4@127.0.0.1:4372",
	}
	listeners := []string{
		":8080",
		":8081",
		":8082",
		":8083",
	}
	l := len(listeners)
	brokers := make([]broker.Broker, l)
	for i := range l {
		brokers[i] = broker.NewBroker(
			broker.WithBrokerID(peers[i]),
			broker.WithListener(server.NetworkTCP, listeners[i]),
			// broker.WithPeers(peers),
		)
		brokers[i].Start()
	}
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		for _, b := range brokers {
			b.Shutdown(context.Background())
		}
		close(done)
	}()
	timeout := time.NewTimer(time.Second * 15)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		slog.Warn("entry server graceful shutdown timeout")
	case <-done:
		slog.Debug("entry server graceful shutdown")
	}
}
