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
	"sutext.github.io/cable/server"
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
		cancel(fmt.Errorf("entry signal received"))
	}()
	peers := []string{"broker1@127.0.0.1:4369", "broker2@127.0.0.1:4370", "broker3@127.0.0.1:4371", "broker4@127.0.0.1:4372"}
	listeners := []broker.Listener{
		{Address: ":8080", Network: server.NetworkTCP},
		{Address: ":8081", Network: server.NetworkTCP},
		{Address: ":8082", Network: server.NetworkTCP},
		{Address: ":8083", Network: server.NetworkTCP},
	}
	brokers := make([]broker.Broker, 4)
	for i := range 4 {
		brokers[i] = broker.NewBroker(
			broker.WithBrokerID(peers[i]),
			broker.WithListeners(listeners[i]),
			broker.WithPeers(peers),
			broker.WithLogger(logger),
		)
		brokers[i].Start()
	}
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		for _, b := range brokers {
			b.Shutdown()
		}
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
