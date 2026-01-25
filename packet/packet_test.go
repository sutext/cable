package packet

import (
	"io"
	"testing"
)

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
		testPacket(t, BigMessage())
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
func BigMessage() Packet {
	data := "eyJhbGciOiJIUzI1NiJ9.eyJpYXQiOjE3NjM1MDA1NTgsImV4cCI6MTc2MzUzNjg1OCwiand0X3VzZXIiOnsiZ3VpZCI6IjM2MjlmZDQwMzVlNDQzMjI5MDVjOTk5NmQ2Y2QyMWM3IiwidXNlcklkIjoyMDU0MjQ2LCJpZGVudGlmaWVyIjoiKzg2LTEyMjAwMDAwMDAwIiwibmlja25hbWUiOiIxMTLnlLXor53pg73lvojlpb3nmoQiLCJhdmF0YXIiOiJodHRwczovL2Nkbi50ZXN0LnV0b3duLmlvL2kvMjAyNDA5MDcvMi83LzkvMjc5ZmQ4N2I5ZDg2NGM0NmEwNGU2MGQyMDNkMTEzYTZfVzEwODBfSDEwODAuanBlZyIsImxhbmciOiJlbiIsInJlZ2lvbiI6IkVOIiwidXNlclR5cGUiOjAsImNvdW50cnkiOiJDTiJ9LCJ1c2VySWQiOiIyMDU0MjQ2In0.x57EFrgc29tk-sv7MJCiwD2988jzeHUenbV9LvCDogQ"
	msg := &Message{
		Qos:     1,
		Dup:     true,
		Kind:    MessageKind(1),
		Payload: []byte(data),
	}
	msg.Set(PropertyClientID, "xxxxxx")
	return msg
}
func testPacket(t *testing.T, p Packet) {
	rw := &ReadWriter{}
	p.Set(PropertyClientID, "test")
	err := WriteTo(rw, p)
	if err != nil {
		t.Error(err)
	}
	newp, err := ReadFrom(rw)
	if err != nil {
		t.Error(err)
	}
	if !p.Equal(newp) {
		t.Error("data packet not equal")
	}
}

type ReadWriter struct {
	data []byte
}

func (w *ReadWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *ReadWriter) Read(p []byte) (n int, err error) {
	l := len(p)
	if l == 0 {
		return 0, nil
	}
	if len(w.data) == 0 {
		return 0, io.EOF
	}
	if l < len(w.data) {
		n = copy(p, w.data[:l])
		w.data = w.data[n:]
		return n, nil
	} else {
		n = copy(p, w.data)
		w.data = nil
		return n, nil
	}
}
