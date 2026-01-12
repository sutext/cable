package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Handler struct {
	b cluster.Broker
}

const (
	MessageKindChat  packet.MessageKind = 1
	MessageKindGroup packet.MessageKind = 2
)

func (h *Handler) OnConnect(c *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}
func (h *Handler) OnClosed(id *packet.Identity) {

}
func (h *Handler) OnMessage(m *packet.Message, id *packet.Identity) error {
	switch m.Kind {
	case MessageKindChat:
		toUserID, ok := m.Get(packet.PropertyUserID)
		if !ok {
			return fmt.Errorf("no user id in message")
		}
		totla, success, err := h.b.SendToUser(context.Background(), toUserID, packet.NewMessage(rand.Int64(), m.Payload))
		if err != nil {
			xlog.Error("Failed to send message to ", xlog.Str("toId", toUserID), xlog.Err(err))
		} else {
			xlog.Info("Success to send message to ", xlog.Str("toId", toUserID), xlog.I32("total", totla), xlog.I32("success", success))
		}
	case MessageKindGroup:
		channel, ok := m.Get(packet.PropertyChannel)
		if !ok {
			return fmt.Errorf("no user id in message")
		}
		totla, success, err := h.b.SendToChannel(context.Background(), channel, packet.NewMessage(rand.Int64(), m.Payload))
		if err != nil {
			xlog.Error("Failed to send message to ", xlog.Channel(channel), xlog.Err(err))
		} else {
			xlog.Info("Success to send message to ", xlog.Channel(channel), xlog.I32("total", totla), xlog.I32("success", success))
		}
	}
	return nil
}

func (h *Handler) GetChannels(uid string) ([]string, error) {
	channels := make([]string, 2)
	for i := range channels {
		channels[i] = fmt.Sprintf("channel%d", rand.IntN(100000))
	}
	return channels, nil
}

type Config struct {
	Debug    bool
	LogLevel slog.Level
}

func EnvConfig() *Config {
	debug := os.Getenv("DEBUG")
	level := os.Getenv("LOG_LEVEL")
	return &Config{
		Debug:    debug == "true",
		LogLevel: xlog.ParseLevel(level),
	}
}

func main() {
	cfg := EnvConfig()
	xlog.SetDefault(xlog.WithLevel(cfg.LogLevel))
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	if cfg.Debug {
		go func() {
			if err := http.ListenAndServe(":6060", nil); err != nil {
				xlog.Error("pprof server start :", xlog.Err(err))
			}
		}()
	}
	go func() {
		<-sigs
		cancel(fmt.Errorf("cable signal received"))
	}()
	h := &Handler{}
	b := cluster.NewBroker(
		cluster.WithHandler(h),
		cluster.WithInitSize(1),
	)
	h.b = b
	if err := b.Start(); err != nil {
		xlog.Error("cable server start :", xlog.Err(err))
		return
	}
	xlog.Info("cable server started")
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		if err := b.Shutdown(context.Background()); err != nil {
			xlog.Error("cable server shutdown :", xlog.Err(err))
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
