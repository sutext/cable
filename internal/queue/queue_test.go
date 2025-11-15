package queue

import (
	"fmt"
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
