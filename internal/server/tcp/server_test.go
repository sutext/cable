package tcp

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTimer(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	timer := time.NewTimer(2 * time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			t.Log("Context done")
		case <-timer.C:
			t.Log("Timer expired")
		}
		t.Log("goroutine quit")
		done <- struct{}{}
	}()
	go func() {
		time.Sleep(1 * time.Second)
		timer.Stop()
	}()
	go func() {
		time.Sleep(1500 * time.Millisecond)
		cancel()
	}()
	<-done
}
func TestChannel(t *testing.T) {
	ctx := context.Background()
	ch := make(chan int)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				t.Log("Context done")
				return
			case i := <-ch:
				t.Log("First goroutine received message", i)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				t.Log("Context done")
				return
			case i := <-ch:
				t.Log("Second goroutine received message", i)
			}
		}
	}()
	go func() {
		for i := 0; i < 10; i++ {
			ch <- i
			time.Sleep(100 * time.Millisecond)
		}
	}()
	// go func() {
	// 	for i := 10; i < 20; i++ {
	// 		ch <- i
	// 		time.Sleep(100 * time.Millisecond)
	// 	}
	// }()
	go func() {
		time.Sleep(5 * time.Second)
		done <- struct{}{}
	}()
	<-done
}
func TestChan(t *testing.T) {
	ch := make(chan int)
	done := make(chan struct{})
	go func() {
		i := <-ch
		t.Log("First goroutine received message", i)
	}()
	go func() {
		i := <-ch
		t.Log("Second goroutine received message", i)
	}()
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(ch)
		time.Sleep(100 * time.Millisecond)
		close(done)

	}()
	go func() {
		<-done
		fmt.Println("Done11111")
	}()
	<-done
	done = nil
	fmt.Println("Done22222")
}
