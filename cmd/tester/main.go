package main

import (
	"context"
	"time"

	"sutext.github.io/cable/xlog"
)

func main() {
	xlog.SetDefault(xlog.WithLevel(xlog.LevelInfo))
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute*4)
	tester := NewTester()
	defer cancel()
	go func() {
		ticker := time.NewTicker(time.Millisecond * 10)
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
