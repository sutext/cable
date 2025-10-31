package keepalive

import (
	"sync"
	"time"
)

type KeepAlive struct {
	mu          *sync.Mutex
	interval    time.Duration
	timeout     time.Duration
	sendFunc    func()
	timeoutFunc func()
	stop        chan struct{}
	pong        chan struct{}
}

func New(interval time.Duration, timeout time.Duration) *KeepAlive {
	return &KeepAlive{
		mu:       new(sync.Mutex),
		interval: interval,
		timeout:  timeout,
	}
}
func (k *KeepAlive) Start() {
	k.mu.Lock()
	k.stop = make(chan struct{})
	k.mu.Unlock()
	go func() {
		ticker := time.NewTicker(k.interval * time.Second)
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
	k.mu.Lock()
	k.pong = make(chan struct{})
	k.mu.Unlock()
	k.sendFunc()
	timer := time.NewTimer(k.timeout * time.Second)
	defer timer.Stop()
	select {
	case <-k.stop:
		return
	case <-k.pong:
		return
	case <-timer.C:
		k.timeoutFunc()
	}
}
