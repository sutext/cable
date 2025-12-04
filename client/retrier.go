package client

import (
	"sync/atomic"
	"time"

	"sutext.github.io/cable/backoff"
)

type Retrier struct {
	limit   int64
	count   atomic.Int64
	backoff backoff.Backoff
	filter  func(error) bool
}

func NewRetrier(limit int64, backoff backoff.Backoff) *Retrier {
	r := &Retrier{
		limit:   limit,
		backoff: backoff,
	}
	return r
}
func (r *Retrier) Filter(f func(error) bool) *Retrier {
	r.filter = f
	return r
}

func (r *Retrier) Retry(reason error) (time.Duration, bool) {
	if r.filter != nil && r.filter(reason) {
		return 0, false
	}
	if r.count.Load() >= r.limit {
		return 0, false
	}
	r.count.Add(1)
	return r.backoff.Next(r.count.Load()), true
}
func (r *Retrier) Reset() {
	r.count.Store(0)
}
