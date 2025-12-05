package queue

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// 添加并发测试
func TestQueue(t *testing.T) {
	mq := New(1000)
	time.AfterFunc(time.Second*10, func() {
		mq.Close()
		fmt.Println("Done")
	})
	go func() {
		for i := range 1000 {
			err := mq.Push(func() {
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
				fmt.Println("Task", i, "done")
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}()
	go func() {
		for i := range 100 {
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			err := mq.Jump(func() {
				fmt.Println("First Task", i, "done")
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}()
	time.Sleep(time.Second * 20)
}
func BenchmarkAddTask(b *testing.B) {
	mq := New(100)
	for b.Loop() {
		mq.Push(func() {
			// do something
		})
	}
}

func BenchmarkAddTaskParallel(b *testing.B) {
	mq := New(1000)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mq.Push(func() {
				// do something
			})
		}
	})
}
