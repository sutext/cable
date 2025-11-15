package keepalive

import (
	"sync"
	"time"
)

type KeepAlive struct {
	mu             *sync.Mutex
	stop           chan struct{}
	pong           chan struct{}
	timeout        time.Duration
	interval       time.Duration
	sendFunc       func()
	timeoutFunc    func()
	lastPacketTime time.Time
}

func New(interval time.Duration, timeout time.Duration) *KeepAlive {
	return &KeepAlive{
		mu:             new(sync.Mutex),
		interval:       interval,
		timeout:        timeout,
		lastPacketTime: time.Now(),
	}
}
func (k *KeepAlive) Start() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stop != nil {
		return
	}
	k.stop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(k.interval)
		for {
			select {
			case <-k.stop:
				ticker.Stop()
				return
			case <-ticker.C:
				go k.sendPing()
			}
		}
	}()
}
func (k *KeepAlive) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.stop != nil {
		close(k.stop)
		k.stop = nil
	}
}
func (k *KeepAlive) UpdateTime() {
	k.lastPacketTime = time.Now()
}
func (k *KeepAlive) PingFunc(f func()) {
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
	if k.lastPacketTime.Add(k.interval).After(time.Now()) {
		return
	}
	k.mu.Lock()
	k.pong = make(chan struct{})
	k.mu.Unlock()
	k.sendFunc()
	select {
	case <-k.stop:
		return
	case <-k.pong:
		return
	case <-time.After(k.timeout):
		k.timeoutFunc()
	}
}
