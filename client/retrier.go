// Package client provides a client implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like automatic reconnection, heartbeat detection, and request-response mechanism.
package client

import (
	"sync/atomic"
	"time"

	"sutext.github.io/cable/backoff"
)

// Retrier implements a retry mechanism with backoff strategy and retry limit.
type Retrier struct {
	limit   int64            // maximum number of retry attempts
	count   atomic.Int64     // current retry count (atomic)
	filter  func(error) bool // error filter to determine if retry is needed
	backoff backoff.Backoff  // backoff strategy for retry intervals
}

// NewRetrier creates a new Retrier instance with the given retry limit and backoff strategy.
//
// Parameters:
// - limit: Maximum number of retry attempts
// - backoff: Backoff strategy to use for calculating retry intervals
//
// Returns:
// - *Retrier: A new Retrier instance
func NewRetrier(limit int64, backoff backoff.Backoff) *Retrier {
	r := &Retrier{
		limit:   limit,
		backoff: backoff,
	}
	return r
}

// Filter sets the error filter function that determines if a retry should be attempted for a given error.
//
// Parameters:
// - f: Function that returns true if retry should be skipped for the given error
//
// Returns:
// - *Retrier: The Retrier instance for method chaining
func (r *Retrier) Filter(f func(error) bool) *Retrier {
	r.filter = f
	return r
}

// Retry checks if a retry should be attempted and returns the retry interval.
//
// Parameters:
// - reason: Error that caused the retry attempt
//
// Returns:
// - time.Duration: Retry interval to wait before next attempt
// - bool: True if retry should be attempted, false otherwise
func (r *Retrier) Retry(reason error) (time.Duration, bool) {
	// Check if error should be filtered (no retry)
	if r.filter != nil && r.filter(reason) {
		return 0, false
	}
	// Check if retry limit has been reached
	if r.count.Load() >= r.limit {
		return 0, false
	}
	// Increment retry count
	r.count.Add(1)
	// Calculate next backoff interval
	return r.backoff.Next(r.count.Load()), true
}

// Reset resets the retry counter to zero.
func (r *Retrier) Reset() {
	r.count.Store(0)
}
