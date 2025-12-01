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
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Handler struct {
	b broker.Broker
}

func (h *Handler) OnConnect(c *packet.Connect) packet.ConnectCode {
	return packet.ConnectAccepted
}
func (h *Handler) OnClosed(id *packet.Identity) {

}
func (h *Handler) OnMessage(m *packet.Message, id *packet.Identity) error {
	var msg string
	if m.Kind == 1 {
		dec := coder.NewDecoder(m.Payload)
		toUserID, err := dec.ReadString()
		if err != nil {
			return err
		}
		msg, err := dec.ReadString()
		if err != nil {
			return err
		}
		totla, success, err := h.b.SendToUser(context.Background(), toUserID, packet.NewMessage([]byte(msg)))
		if err != nil {
			xlog.Errorf("send message to ", slog.String("toId", toUserID), slog.String("msg", msg), slog.Any("error", err))
		} else {
			xlog.Info("send message to ", slog.String("toId", toUserID), slog.String("msg", msg), slog.Uint64("total", totla), slog.Uint64("success", success))
		}
	}
	xlog.Info("receive message from ", slog.String("fromId", id.UserID), slog.String("msg", msg))
	return nil
}
func (h *Handler) GetChannels(uid string) ([]string, error) {
	return []string{"test"}, nil
}

func main() {
	xlog.SetDefault(xlog.WithLevel(xlog.LevelWarn))
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := http.ListenAndServe(":6060", nil); err != nil {
			xlog.Error("pprof server start :", err)
		}
	}()
	go func() {
		<-sigs
		cancel(fmt.Errorf("cable signal received"))
	}()
	h := &Handler{}
	b := broker.NewBroker(broker.WithHandler(h))
	h.b = b
	if err := b.Start(); err != nil {
		xlog.Error("cable server start :", err)
		return
	}
	xlog.Info("cable server started")
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		if err := b.Shutdown(context.Background()); err != nil {
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
