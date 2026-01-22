package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sutext.github.io/cable/xlog"
)

var configFile = "config.yaml"

func init() {
	flag.StringVar(&configFile, "config", "config.yaml", "config file path")
}
func main() {
	flag.Parse()
	conf, err := readConfig(configFile)
	if err != nil {
		panic(err)
	}
	var logger *xlog.Logger
	if conf.LogFormat == "json" {
		logger = xlog.NewJSON(conf.Level().Level())
	} else {
		logger = xlog.NewText(conf.Level().Level())
	}
	xlog.SetDefault(logger)
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	if conf.Pprof {
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
	boot := newBooter(conf)
	boot.Start()
	xlog.Info("cable server started")
	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		if err := boot.Shutdown(context.Background()); err != nil {
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
