package inflight

import (
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/packet"
)

type message struct {
	message  *packet.Message
	attempts int
	lastTime time.Time
}
type Inflight struct {
	mu          sync.Mutex
	interval    time.Duration
	maxAttempts int
	messages    sync.Map
	action      func(*packet.Message)
	count       atomic.Int64
	stop        chan struct{}
}

func New(action func(*packet.Message)) *Inflight {
	return &Inflight{
		action:      action,
		interval:    time.Second * 3,
		maxAttempts: 3,
	}
}

func (i *Inflight) Start() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.stop != nil {
		return
	}
	i.stop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(i.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if i.Count() == 0 {
					continue
				}
				i.messages.Range(func(key, value any) bool {
					msg := value.(*message)
					if time.Since(msg.lastTime) > i.interval {
						msg.attempts++
						msg.lastTime = time.Now()
						if msg.attempts >= i.maxAttempts {
							i.messages.Delete(key)
						} else {
							msg.message.Dup = true
							i.action(msg.message)
						}
					}
					return true
				})
			case <-i.stop:
				return
			}
		}
	}()
}

func (i *Inflight) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.stop == nil {
		return
	}
	close(i.stop)
	i.stop = nil
}

func (i *Inflight) Add(m *packet.Message) {
	i.messages.Store(m.ID, &message{
		message:  m,
		attempts: 0,
		lastTime: time.Now(),
	})
	i.count.Add(1)
}

func (i *Inflight) Remove(id int64) {
	i.messages.Delete(id)
	i.count.Add(-1)
}

func (i *Inflight) Count() int64 {
	return i.count.Load()
}
