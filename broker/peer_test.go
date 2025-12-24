package broker

import (
	"context"
	"testing"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

func BenchmarkPeerInspect(b *testing.B) {
	client := newPeerClient("1", "127.0.0.1:4567", xlog.Defualt())
	client.connect()
	defer client.Close()
	b.Run("GrpcSend", func(b *testing.B) {
		_, _, err := client.sendMessage(context.Background(), packet.NewMessage([]byte("")), "", 0)
		if err != nil {
			b.Fatal(err)
		}
	})
}
