package inflight

import (
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
)

type message struct {
	message  *packet.Message
	attempts int
	lastTime time.Time
}
type Inflight struct {
	interval    time.Duration
	maxAttempts int
	messages    safe.XMap[int64, *message]
	action      func(*packet.Message)
	stopChan    chan struct{}
	closed      atomic.Bool
}

func New(action func(*packet.Message)) *Inflight {
	i := &Inflight{
		action:      action,
		interval:    time.Second * 3,
		maxAttempts: 3,
		stopChan:    make(chan struct{}),
	}
	go i.run()
	return i
}

func (i *Inflight) run() {
	ticker := time.NewTicker(i.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			i.messages.Range(func(key int64, msg *message) bool {
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
		case <-i.stopChan:
			return
		}
	}
}
func (i *Inflight) Close() {
	if i.closed.CompareAndSwap(false, true) {
		close(i.stopChan)
	}
}
func (i *Inflight) Add(m *packet.Message) {
	i.messages.Set(m.ID, &message{
		message:  m,
		attempts: 0,
		lastTime: time.Now(),
	})
}

func (i *Inflight) Remove(id int64) {
	i.messages.Delete(id)
}

func (i *Inflight) Len() int64 {
	return int64(i.messages.Len())
}
