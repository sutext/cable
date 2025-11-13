package queue

import (
	"math/rand"
	"testing"
	"time"
)

// 添加并发测试
func TestQueue(t *testing.T) {
	mq := New(10, 10)
	go func() {
		time.Sleep(time.Millisecond * 1000)
		mq.Stop()
	}()
	go func() {
		for range 1000 {
			mq.Push(func() {
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			})
		}
	}()
	select {}
}
