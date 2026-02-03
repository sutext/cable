package main

import (
	"context"
	"flag"
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
		logger = xlog.NewJSON(conf.Level())
	} else {
		logger = xlog.NewText(conf.Level())
	}
	xlog.SetDefault(logger)
	if conf.Pprof {
		go func() {
			if err := http.ListenAndServe(":6060", nil); err != nil {
				xlog.Error("pprof server start :", xlog.Err(err))
			}
		}()
	}
	boot := newBooter(conf)
	boot.Start()
	xlog.Info("cable server started")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	xlog.Info("cable server shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	if err := boot.Shutdown(ctx); err != nil {
		xlog.Error("cable server shutdown :", xlog.Err(err))
	}
	xlog.Info("cable server shutdown gracefully")
}
