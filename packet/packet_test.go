package packet

import (
	"fmt"
	"io"
	"testing"
)

func TestPacket(t *testing.T) {
	// identity := Identity{"1", "2", "tok"}
	// testp(t, NewConnect(&identity))
	// testp(t, NewConnack(0))
	// testp(t, SmallMessage())
	testp(t, BigMessage())
	// testp(t, NewPing())
	// testp(t, NewPong())
	// testp(t, NewClose(0))
	// testp(t, NewRequest("test"))
	// testp(t, NewResponse(1))
}
func SmallMessage() Packet {
	return NewMessage([]byte("hello world"))
}
func BigMessage() Packet {
	data := make([]byte, 0xfff)
	return NewMessage(data)
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
