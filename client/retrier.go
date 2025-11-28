package client

import (
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/backoff"
)

type Retrier struct {
	mu      *sync.Mutex
	limit   int64
	count   atomic.Int64
	backoff backoff.Backoff
	filter  func(error) bool
	stop    chan struct{}
}

func NewRetrier(limit int64, backoff backoff.Backoff) *Retrier {
	r := &Retrier{
		mu:      &sync.Mutex{},
		limit:   limit,
		backoff: backoff,
	}
	return r
}
func (r *Retrier) Filter(f func(error) bool) *Retrier {
	r.filter = f
	return r
}

func (r *Retrier) can(reason error) (time.Duration, bool) {
	if r.filter != nil && r.filter(reason) {
		return 0, false
	}
	if r.count.Load() >= r.limit {
		return 0, false
	}
	r.count.Add(1)
	return r.backoff.Next(r.count.Load()), true
}
func (r *Retrier) cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stop == nil {
		return
	}
	r.count.Store(0)
	close(r.stop)
	r.stop = nil
}

func (r *Retrier) retry(delay time.Duration, fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stop != nil {
		return
	}
	r.stop = make(chan struct{})
	go func() {
		select {
		case <-time.After(delay):
			fn()
		case <-r.stop:
			return
		}
	}()
}
