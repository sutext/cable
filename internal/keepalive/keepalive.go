package keepalive

import (
	"sync"
	"sync/atomic"
	"time"
)

type KeepAlive struct {
	mu             sync.Mutex
	stop           chan struct{}
	pong           chan struct{}
	closed         atomic.Bool
	timeout        time.Duration
	interval       time.Duration
	sendFunc       func() error
	timeoutFunc    func()
	lastPacketTime atomic.Int64
}

func New(interval time.Duration, timeout time.Duration) *KeepAlive {
	k := &KeepAlive{
		interval: interval,
		timeout:  timeout,
		stop:     make(chan struct{}),
	}
	k.lastPacketTime.Store(time.Now().UnixMicro())
	go k.run()
	return k
}
func (k *KeepAlive) run() {
	ticker := time.NewTicker(k.interval)
	defer ticker.Stop()
	for {
		select {
		case <-k.stop:
			return
		case <-ticker.C:
			k.sendPing()
		}
	}
}
func (k *KeepAlive) Close() {
	if k.closed.CompareAndSwap(false, true) {
		close(k.stop)
	}
}
func (k *KeepAlive) UpdateTime() {
	k.lastPacketTime.Store(time.Now().UnixNano())
}
func (k *KeepAlive) PingFunc(f func() error) {
	k.sendFunc = f
}
func (k *KeepAlive) HandlePong() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.pong != nil {
		close(k.pong)
		k.pong = nil
	}
}
func (k *KeepAlive) TimeoutFunc(f func()) {
	k.timeoutFunc = f
}
func (k *KeepAlive) sendPing() {
	if time.Duration(time.Now().UnixNano()-k.lastPacketTime.Load()) < k.interval {
		return
	}
	k.mu.Lock()
	k.pong = make(chan struct{})
	k.mu.Unlock()
	if err := k.sendFunc(); err != nil {
		return
	}
	timer := time.NewTimer(k.timeout)
	select {
	case <-k.pong:
		timer.Stop()
		return
	case <-timer.C:
		k.timeoutFunc()
	}
}
