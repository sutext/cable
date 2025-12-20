package keepalive

import (
	"sync/atomic"
	"time"
)

type pinger interface {
	SendPing()
}
type KeepAlive struct {
	pinger         pinger
	stoped         atomic.Bool
	interval       time.Duration
	lastPacketTime atomic.Int64
}

func New(interval time.Duration, pinger pinger) *KeepAlive {
	k := &KeepAlive{
		interval: interval,
		pinger:   pinger,
	}
	k.lastPacketTime.Store(time.Now().UnixNano())
	return k
}
func (k *KeepAlive) run() {
	if k.stoped.Load() {
		return
	}
	k.sendPing()
	time.AfterFunc(k.interval, k.run)
}
func (k *KeepAlive) Start() {
	if k.stoped.CompareAndSwap(true, false) {
		go k.run()
	}
}
func (k *KeepAlive) Stop() {
	k.stoped.Store(true)
}
func (k *KeepAlive) UpdateTime() {
	k.lastPacketTime.Store(time.Now().UnixNano())
}

func (k *KeepAlive) sendPing() {
	if time.Duration(time.Now().UnixNano()-k.lastPacketTime.Load()) < k.interval {
		return
	}
	k.pinger.SendPing()
}
