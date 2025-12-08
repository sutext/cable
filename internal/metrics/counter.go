package mertrics

import "sync/atomic"

type Counter interface {
	Inc()
	Count() uint64
	Snapshot() uint64
}

func NewCounter() Counter {
	return &counter{}
}

type counter struct {
	count atomic.Uint64
}

func (c *counter) Inc() {
	c.count.Add(1)
}

func (c *counter) Count() uint64 {
	return c.count.Load()
}

func (c *counter) Snapshot() uint64 {
	return c.count.Load()
}
