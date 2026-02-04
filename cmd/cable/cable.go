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
	logger := conf.GetLogger()
	if conf.Pprof {
		go func() {
			if err := http.ListenAndServe(":6060", nil); err != nil {
				logger.Error("pprof server start :", xlog.Err(err))
			}
		}()
	}
	boot := newBooter(conf)
	boot.Start()
	logger.Info("cable server started")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	logger.Info("cable server shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	if err := boot.Shutdown(ctx); err != nil {
		logger.Error("cable server shutdown :", xlog.Err(err))
	}
	logger.Info("cable server shutdown gracefully")
}
