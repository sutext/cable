package packet

import (
	"testing"
)

func BenchmarkPacket(b *testing.B) {
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			testPacket(b, NewConnack(0))
			testPacket(b, NewConnack(0))
			testPacket(b, SmallMessage())
			testPacket(b, BigMessage(0x3f))
			testPacket(b, NewPing())
			testPacket(b, NewPong())
			testPacket(b, NewClose(0))
			testPacket(b, &Request{Method: "test"})
		}
	})
}
func TestPacket(t *testing.T) {
	t.Run("Connect", func(t *testing.T) {
		identity := Identity{"1", "2", "tok"}
		testPacket(t, NewConnect(&identity))
	})
	t.Run("Connack", func(t *testing.T) {
		testPacket(t, NewConnack(0))
	})
	t.Run("Message", func(t *testing.T) {
		testPacket(t, SmallMessage())
	})
	t.Run("BigMessage", func(t *testing.T) {
		testPacket(t, BigMessage(0x3_ff+1))
		testPacket(t, BigMessage(0x3_ffff+1))
		testPacket(t, BigMessage(0x3_ffff_ff+1))
		// testPacket(t, BigMessage(0x3_ffff_ffff))
	})
	t.Run("Ping", func(t *testing.T) {
		ping := NewPing()
		ping.Set(PropertyClientID, "pingping")
		testPacket(t, ping)
	})
	t.Run("Pong", func(t *testing.T) {
		pong := NewPong()
		pong.Set(PropertyClientID, "pongpong")
		testPacket(t, pong)
	})
	t.Run("Close", func(t *testing.T) {
		testPacket(t, NewClose(0))
	})
	t.Run("Request", func(t *testing.T) {
		testPacket(t, &Request{Method: "test"})
	})
	t.Run("Response", func(t *testing.T) {
		testPacket(t, (&Request{Method: "test"}).Response(StatusOK))
	})
}
func SmallMessage() Packet {
	return &Message{
		Payload: []byte("hello world"),
	}
}
func BigMessage(len int) Packet {
	msg := &Message{
		Kind:    MessageKind(1),
		Payload: make([]byte, len),
	}
	msg.Set(PropertyClientID, "xxxxxx")
	return msg
}

type errorFunc interface {
	Error(args ...any)
}

func testPacket(t errorFunc, p Packet) {
	p.Set(PropertyClientID, "test")
	data, err := Marshal(p)
	if err != nil {
		t.Error(err)
	}
	newp, err := Unmarshal(data)
	if err != nil {
		t.Error(err)
	}
	if !p.Equal(newp) {
		t.Error("data packet not equal")
	}
}
