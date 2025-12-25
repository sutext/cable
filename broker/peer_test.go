package broker

import (
	"context"
	"testing"

	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

func BenchmarkPeerInspect(b *testing.B) {
	client := newPeerClient("cable-cluster-0", "172.16.2.123:4567", xlog.Defualt())
	client.connect()
	defer client.Close()
	b.Run("GrpcSend", func(b *testing.B) {
		for b.Loop() {
			client.sendMessage(context.Background(), packet.NewMessage([]byte("hello")), "aaa", 0)
		}
	})
}
