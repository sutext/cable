package packet

import (
	"fmt"
	"io"
	"testing"
)

func TestPacket(t *testing.T) {
	identity := Identity{"1", "2", "tok"}
	testp(t, NewConnect(&identity))
	testp(t, NewConnack(0))
	testp(t, SmallMessage())
	testp(t, BigMessage())
	testp(t, NewPing())
	testp(t, NewPong())
	testp(t, NewClose(0))
	testp(t, NewRequest("test"))
	testp(t, NewResponse(1))
}
func SmallMessage() Packet {
	return NewMessage([]byte("hello world"))
}
func BigMessage() Packet {
	data := "eyJhbGciOiJIUzI1NiJ9.eyJpYXQiOjE3NjM1MDA1NTgsImV4cCI6MTc2MzUzNjg1OCwiand0X3VzZXIiOnsiZ3VpZCI6IjM2MjlmZDQwMzVlNDQzMjI5MDVjOTk5NmQ2Y2QyMWM3IiwidXNlcklkIjoyMDU0MjQ2LCJpZGVudGlmaWVyIjoiKzg2LTEyMjAwMDAwMDAwIiwibmlja25hbWUiOiIxMTLnlLXor53pg73lvojlpb3nmoQiLCJhdmF0YXIiOiJodHRwczovL2Nkbi50ZXN0LnV0b3duLmlvL2kvMjAyNDA5MDcvMi83LzkvMjc5ZmQ4N2I5ZDg2NGM0NmEwNGU2MGQyMDNkMTEzYTZfVzEwODBfSDEwODAuanBlZyIsImxhbmciOiJlbiIsInJlZ2lvbiI6IkVOIiwidXNlclR5cGUiOjAsImNvdW50cnkiOiJDTiJ9LCJ1c2VySWQiOiIyMDU0MjQ2In0.x57EFrgc29tk-sv7MJCiwD2988jzeHUenbV9LvCDogQ"
	msg := NewMessage([]byte(data))
	msg.PropSet(PropertyUDPConnID, "xxxxxx")
	fmt.Println(msg)
	return msg
}
func testp(t *testing.T, p Packet) {
	rw := &ReadWriter{}
	err := WriteTo(rw, p)
	if err != nil {
		t.Error(err)
	}
	newp, err := ReadFrom(rw)
	if err != nil {
		t.Error(err)
	}
	if !p.Equal(newp) {
		fmt.Printf("old packet: %v\n", p)
		fmt.Printf("new packet: %v\n", newp)
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
