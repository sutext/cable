package main

import (
	"context"
	"fmt"
	"math/rand/v2"

	"sutext.github.io/cable/api"
	"sutext.github.io/cable/xlog"
)

func main() {
	client := api.NewClient("172.16.2.123:1887")
	client.Connect()
	ctx := context.Background()
	max := 100000
	for i := range max {
		uid := fmt.Sprintf("u%d", i)
		err := client.JoinChannel(ctx, uid, randomChannelIDs(max))
		if err != nil {
			xlog.Error("join channel failed", xlog.Str("uid", uid), xlog.Err(err))
		} else {
			xlog.Info("join channel success", xlog.Str("uid", uid))
		}
	}
}

func randomUserID(max int) string {
	return fmt.Sprintf("u%d", rand.IntN(max))
}
func randomChannelIDs(max int) map[string]string {
	ch1 := fmt.Sprintf("channel%d", rand.IntN(max))
	ch2 := fmt.Sprintf("channel%d", rand.IntN(max))
	// ch3 := fmt.Sprintf("channel%d", rand.IntN(max))
	return map[string]string{
		ch1: "",
		ch2: "",
		// ch3: "",
	}
}
