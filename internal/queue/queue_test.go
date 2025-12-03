package queue

import (
	"math/rand"
	"testing"
	"time"
)

// 添加并发测试
func TestQueue(t *testing.T) {
	mq := New(1024)
	go func() {
		time.Sleep(time.Millisecond * 1323)
		mq.Close()
	}()
	go func() {
		for range 1000 {
			mq.AddTask(func() {
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			})
		}
	}()
	time.Sleep(time.Second * 3)
}
func BenchmarkAddTask(b *testing.B) {
	mq := New(1000)
	for b.Loop() {
		mq.AddTask(func() {
			// do something
		})
	}
}

func BenchmarkAddTaskParallel(b *testing.B) {
	mq := New(1000)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mq.AddTask(func() {
				// do something
			})
		}
	})
}
