package mq

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// 添加并发测试
func TestQueue(t *testing.T) {
	mq := NewLQueue()
	time.AfterFunc(time.Second*2, func() {
		mq.Pause()
		fmt.Println("Pause")
		time.AfterFunc(time.Second*2, func() {
			mq.Resume()
			fmt.Println("Resume")
			time.AfterFunc(time.Second*2, func() {
				mq.Close()
				fmt.Println("Close")
			})
		})
	})
	go func() {
		for i := range 1000 {
			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			err := mq.AddTask(func() {
				fmt.Println("Task", i, "done")
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}()
	time.Sleep(time.Second * 20)
}

func BenchmarkAddTask(b *testing.B) {
	b.Run("slice_queue", func(b *testing.B) {
		mq := NewQueue(1000)
		for b.Loop() {
			mq.Push(func() {
				// do something
			})
		}
	})
	b.Run("list_queue", func(b *testing.B) {
		mq := NewLQueue()
		for b.Loop() {
			mq.AddTask(func() {
				// do something
			})
		}
	})
}

func BenchmarkAddTaskParallel(b *testing.B) {
	mq := NewQueue(1000)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mq.Push(func() {
				// do something
			})
		}
	})
}
