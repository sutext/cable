package metrics

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Meters count events to produce exponentially-weighted moving average rates
// at one-, five-, and fifteen-minutes and a mean rate.
type Meter interface {
	Count() int64
	Mark(int64)
	Rate() float64
	RateMean() float64
	Snapshot() Meter
	Stop()
}

// NewMeter constructs a new StandardMeter and launches a goroutine.
// Be sure to call Stop() once the meter is of no use to allow for garbage collection.
func NewMeter() Meter {
	m := newStandardMeter()
	arbiter.Lock()
	defer arbiter.Unlock()
	arbiter.meters[m] = struct{}{}
	if !arbiter.started {
		arbiter.started = true
		go arbiter.tick()
	}
	return m
}

// MeterSnapshot is a read-only copy of another Meter.
type MeterSnapshot struct {
	count          int64
	rate, rateMean uint64
}

// Count returns the count of events at the time the snapshot was taken.
func (m *MeterSnapshot) Count() int64 { return m.count }

// Mark panics.
func (*MeterSnapshot) Mark(n int64) {
	panic("Mark called on a MeterSnapshot")
}

// Rate1 returns the one-minute moving average rate of events per second at the
// time the snapshot was taken.
func (m *MeterSnapshot) Rate() float64 { return math.Float64frombits(m.rate) }

// RateMean returns the meter's mean rate of events per second at the time the
// snapshot was taken.
func (m *MeterSnapshot) RateMean() float64 { return math.Float64frombits(m.rateMean) }

// Snapshot returns the snapshot.
func (m *MeterSnapshot) Snapshot() Meter { return m }

// Stop is a no-op.
func (m *MeterSnapshot) Stop() {}

// NilMeter is a no-op Meter.
type NilMeter struct{}

// StandardMeter is the standard implementation of a Meter.
type StandardMeter struct {
	ewma      EWMA
	stopped   uint32
	startTime time.Time
	snapshot  *MeterSnapshot
}

func newStandardMeter() *StandardMeter {
	return &StandardMeter{
		snapshot:  &MeterSnapshot{},
		ewma:      NewEWMA1(),
		startTime: time.Now(),
	}
}

// Stop stops the meter, Mark() will be a no-op if you use it after being stopped.
func (m *StandardMeter) Stop() {
	if atomic.CompareAndSwapUint32(&m.stopped, 0, 1) {
		arbiter.Lock()
		delete(arbiter.meters, m)
		arbiter.Unlock()
	}
}

// Count returns the number of events recorded.
func (m *StandardMeter) Count() int64 {
	return atomic.LoadInt64(&m.snapshot.count)
}

// Mark records the occurance of n events.
func (m *StandardMeter) Mark(n int64) {
	if atomic.LoadUint32(&m.stopped) == 1 {
		return
	}
	atomic.AddInt64(&m.snapshot.count, n)
	m.ewma.Update(n)
	m.updateSnapshot()
}

// Rate1 returns the one-minute moving average rate of events per second.
func (m *StandardMeter) Rate() float64 {
	return math.Float64frombits(atomic.LoadUint64(&m.snapshot.rate))
}

// RateMean returns the meter's mean rate of events per second.
func (m *StandardMeter) RateMean() float64 {
	return math.Float64frombits(atomic.LoadUint64(&m.snapshot.rateMean))
}

// Snapshot returns a read-only copy of the meter.
func (m *StandardMeter) Snapshot() Meter {
	copiedSnapshot := MeterSnapshot{
		count:    atomic.LoadInt64(&m.snapshot.count),
		rate:     atomic.LoadUint64(&m.snapshot.rate),
		rateMean: atomic.LoadUint64(&m.snapshot.rateMean),
	}
	return &copiedSnapshot
}

func (m *StandardMeter) updateSnapshot() {
	rate := math.Float64bits(m.ewma.Rate())
	rateMean := math.Float64bits(float64(m.Count()) / time.Since(m.startTime).Seconds())
	atomic.StoreUint64(&m.snapshot.rate, rate)
	atomic.StoreUint64(&m.snapshot.rateMean, rateMean)
}

func (m *StandardMeter) tick() {
	m.ewma.Tick()
	m.updateSnapshot()
}

// meterArbiter ticks meters every 5s from a single goroutine.
// meters are references in a set for future stopping.
type meterArbiter struct {
	sync.RWMutex
	started bool
	meters  map[*StandardMeter]struct{}
	ticker  *time.Ticker
}

var arbiter = meterArbiter{ticker: time.NewTicker(5e9), meters: make(map[*StandardMeter]struct{})}

// Ticks meters on the scheduled interval
func (ma *meterArbiter) tick() {
	for range ma.ticker.C {
		ma.RLock()
		defer ma.RUnlock()
		for meter := range ma.meters {
			meter.tick()
		}
	}
}
