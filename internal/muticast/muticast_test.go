package muticast

import (
	"testing"
	"time"
)

func TestMuticast(t *testing.T) {
	ch := make(chan int)
	time.AfterFunc(time.Second*5, func() {
		close(ch)
	})
	for i := range ch {
		t.Log(i)
	}
	t.Log("done")
}
