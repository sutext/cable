package queue

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// 添加并发测试
func TestQueue(t *testing.T) {
	mq := New(1000)
	time.AfterFunc(time.Second*2, func() {
		mq.Close()
	})
	go func() {
		for i := range 1000 {
			err := mq.Push(context.Background(), false, func() {
				time.Sleep(time.Duration(100 * time.Millisecond))
				fmt.Println("Task", i, "done")
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}()
	go func() {
		for i := range 100 {
			time.Sleep(time.Duration(1000 * time.Millisecond))
			err := mq.Push(context.Background(), true, func() {
				fmt.Println("First Task", i, "done")
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}()
	time.Sleep(time.Second * 20)
}
func TestChannelClose(t *testing.T) {
	s := &struct{ ch chan int }{
		ch: make(chan int, 1000),
	}
	// go func() {
	// 	for {
	// 		select {
	// 		case i, ok := <-s.ch:
	// 			fmt.Println("receive", i, ok)
	// 			time.Sleep(time.Duration(10 * time.Millisecond))
	// 		default:
	// 			fmt.Println("default")
	// 		}
	// 		time.Sleep(time.Duration(10 * time.Millisecond))
	// 	}
	// }()
	go func() {
		for i := range s.ch {
			fmt.Println("receive", i)
			time.Sleep(time.Duration(10 * time.Millisecond))
		}
	}()
	for i := range 1000 {
		s.ch <- i
	}
	time.AfterFunc(time.Millisecond*2000, func() {
		close(s.ch)
		s.ch = nil
		fmt.Println("Closed")
	})
	time.Sleep(time.Second * 5)
}

// 性能测试
func BenchmarkAddTask(b *testing.B) {
	mq := New(100)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	for b.Loop() {
		mq.Push(ctx, false, func() {
			// do something
		})
	}
}

func BenchmarkAddTaskParallel(b *testing.B) {
	mq := New(1000)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mq.Push(ctx, false, func() {
				// do something
			})
		}
	})
}
