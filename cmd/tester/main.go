package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"sutext.github.io/cable/xlog"
)

func main() {
	xlog.SetDefault(xlog.WithLevel(xlog.LevelInfo))
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*getFrequency()*getMaxConns())
	tester := NewTester()
	defer cancel()
	go func() {
		ticker := time.NewTicker(time.Millisecond * getFrequency())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tester.addClient()
			}
		}
	}()
	select {}
}
func getMaxConns() time.Duration {
	count := os.Getenv("MAX_CONNS")
	if count == "" {
		return 5
	}
	i, err := strconv.ParseInt(count, 10, 64)
	if err != nil {
		return 25000
	}
	return time.Duration(i)
}
func getFrequency() time.Duration {
	freq := os.Getenv("FREQ")
	if freq == "" {
		return 100
	}
	d, err := strconv.ParseInt(freq, 10, 64)
	if err != nil {
		return 100
	}
	return time.Duration(d)
}
