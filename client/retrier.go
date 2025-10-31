package client

import (
	"sync"
	"time"

	"sutext.github.io/cable/internal/backoff"
)

type Retrier struct {
	mu      *sync.Mutex
	limit   int
	count   int
	backoff backoff.Backoff
	filter  func(error) bool
	stop    chan struct{}
}

func NewRetrier(limit int, backoff backoff.Backoff) *Retrier {
	return &Retrier{
		mu:      &sync.Mutex{},
		limit:   limit,
		count:   0,
		backoff: backoff,
	}
}
func (r *Retrier) Filter(f func(error) bool) *Retrier {
	r.filter = f
	return r
}

func (r *Retrier) can(reason error) (time.Duration, bool) {
	if r.filter != nil && r.filter(reason) {
		return 0, false
	}
	if r.count >= r.limit {
		return 0, false
	}
	r.count++
	return r.backoff.Next(r.count), true
}
func (r *Retrier) cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count = 0
	if r.stop != nil {
		close(r.stop)
		r.stop = nil
	}
}

func (r *Retrier) retry(delay time.Duration, fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stop = make(chan struct{})
	timer := time.NewTimer(delay)
	go func() {
		defer timer.Stop()
		select {
		case <-timer.C:
			fn()
		case <-r.stop:
			return
		}
	}()
}
