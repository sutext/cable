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
func TestChan(t *testing.T) {
	ch := make(chan int, 5)
	go func() {
		for i := range 10 {
			ch <- i
			fmt.Println("Sent:", i)
		}
		time.Sleep(10 * time.Second)
		close(ch)
		fmt.Println("Channel closed")
	}()

	go func() {
		for {
			value, ok := <-ch
			if ok {
				fmt.Println("Received:", value)
			} else {
				fmt.Println("Channel closed and no more data to receive.")
				return
			}
		}
	}()
	time.Sleep(11 * time.Second)
}
