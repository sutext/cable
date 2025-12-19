package safe

import (
	"sync/atomic"
)

var ErrChanClosed = errChannelClosed{}

type errChannelClosed struct{}

func (e errChannelClosed) Error() string {
	return "channel is closed"
}

type Chan[T any] struct {
	closed atomic.Bool
	ch     chan T
}

func NewChan[T any](size ...int) *Chan[T] {
	l := 0
	if len(size) == 1 {
		l = size[0]
	} else {
		panic("invalid parameter")
	}
	return &Chan[T]{
		ch: make(chan T, l),
	}
}

func (c *Chan[T]) Send(v T) error {
	if c.closed.Load() {
		return ErrChanClosed
	}
	c.ch <- v
	return nil
}
func (c *Chan[T]) C() chan T {
	return c.ch
}
func (c *Chan[T]) Recv() (t T, err error) {
	if c.closed.Load() {
		return t, ErrChanClosed
	}
	return <-c.ch, nil
}
func (c *Chan[T]) IsClosed() bool {
	return c.closed.Load()
}
func (c *Chan[T]) Close() {
	if c.closed.CompareAndSwap(false, true) {
		close(c.ch)
		c.ch = nil
	}
}
