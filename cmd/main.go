package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable"
	"sutext.github.io/cable/logger"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/server"
)

func main() {
	ctx := context.Background()
	logger := logger.New(logger.LevelDebug, logger.FormatJSON)

	s, err := cable.NewServer(server.WithTCP(":8080"))
	// s, err := entry.NewServer(entry.QUIC(&quic.Config{
	// 	TLSConfig:            &tls.Config{InsecureSkipVerify: true},
	// 	MaxBidiRemoteStreams: 1,
	// 	MaxIdleTimeout:       5 * time.Second,
	// 	MaxUniRemoteStreams:  1,
	// }), ":8080")
	if err != nil {
		logger.Error("listen", "error", err)
		os.Exit(1)
	}

	s.OnAuth(func(p *packet.Identity) error {
		return nil
	})
	s.OnData(func(cid string, p *packet.DataPacket) (*packet.DataPacket, error) {
		return nil, nil
	})
	logger.InfoContext(ctx, "entry server start")
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		logger.InfoContext(ctx, "entry signal received")
		cancel(fmt.Errorf("entry signal received"))
	}()
	go func() {
		if err := s.Serve(); err != nil {
			cancel(err)
		}
	}()
	<-ctx.Done()
	logger.InfoContext(ctx, "entry server start graceful shutdown")
	done := make(chan struct{})
	go func() {
		s.Shutdown(ctx)
		close(done)
	}()
	timeout := time.NewTimer(time.Second * 15)
	defer timeout.Stop()
	select {
	case <-timeout.C:
		logger.WarnContext(ctx, "entry server graceful shutdown timeout")
	case <-done:
		logger.DebugContext(ctx, "entry server graceful shutdown")
	}
}
