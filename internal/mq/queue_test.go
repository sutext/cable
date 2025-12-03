package mq

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// 添加并发测试
func TestQueue(t *testing.T) {
	mq := NewQueue(5)
	time.AfterFunc(time.Second*2, func() {
		mq.Pause()
		fmt.Println("Pause")
		time.AfterFunc(time.Second*2, func() {
			mq.Resume()
			fmt.Println("Resume")
			time.AfterFunc(time.Second*2, func() {
				mq.Clear()
				fmt.Println("Clear")
			})
		})
	})
	go func() {
		for i := range 1000 {
			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			mq.AddTask(func() {
				fmt.Println("Task", i, "done")
			})
		}
	}()
	time.Sleep(time.Second * 20)
}

func BenchmarkAddTask(b *testing.B) {
	mq := NewQueue(1000)
	for b.Loop() {
		mq.AddTask(func() {
			// do something
		})
	}
}

func BenchmarkAddTaskParallel(b *testing.B) {
	mq := NewQueue(1000)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mq.AddTask(func() {
				// do something
			})
		}
	})
}
