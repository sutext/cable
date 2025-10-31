package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrStop = errors.New("push failed: queue is stopped")
	ErrFull = errors.New("push failed: queue is full")
)

const (
	pushWait = time.Second * 3
)

type Queue struct {
	wg        sync.WaitGroup
	count     atomic.Int64
	taskChan  chan func()
	isStopped atomic.Bool
}

// Create a new instance of Queue
func NewQueue(workerCount int, bufferSize int) *Queue {
	if workerCount < 1 || bufferSize < 1 {
		panic("workerCount and bufferSize must be greater than 0")
	}
	mq := &Queue{
		taskChan: make(chan func(), bufferSize),
	}
	for range workerCount {
		mq.wg.Go(func() {
			for task := range mq.taskChan {
				task()
			}
		})
	}
	return mq
}

// Push submits a function to the queue.
// Returns an error if the queue is stopped or if the queue is full.
func (mq *Queue) Push(task func()) error {
	mq.count.Add(1)
	defer mq.count.Add(-1)
	if mq.isStopped.Load() {
		return ErrStop
	}
	timer := time.NewTimer(pushWait)
	defer timer.Stop()
	select {
	case mq.taskChan <- task:
		return nil
	case <-timer.C: // Timeout to prevent deadlock/blocking
		return ErrFull
	}
}

func (mq *Queue) PushCtx(ctx context.Context, task func()) error {
	mq.count.Add(1)
	defer mq.count.Add(-1)
	if mq.isStopped.Load() {
		return ErrStop
	}
	select {
	case mq.taskChan <- task:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (mq *Queue) BatchPushCtx(ctx context.Context, tasks ...func()) (int, error) {
	mq.count.Add(1)
	defer mq.count.Add(-1)
	if mq.isStopped.Load() {
		return 0, ErrStop
	}
	for i := range tasks {
		select {
		case <-ctx.Done():
			return i, context.Cause(ctx)
		case mq.taskChan <- tasks[i]:
		}
	}
	return len(tasks), nil
}

func (mq *Queue) NotWaitPush(task func()) error {
	mq.count.Add(1)
	defer mq.count.Add(-1)
	if mq.isStopped.Load() {
		return ErrStop
	}
	select {
	case mq.taskChan <- task:
		return nil
	default:
		return ErrFull
	}
}

// Stop is used to terminate the internal goroutines and close the channel.
func (mq *Queue) Stop() {
	if !mq.isStopped.CompareAndSwap(false, true) {
		return
	}
	mq.waitSafeClose()
	close(mq.taskChan)
	mq.wg.Wait()
}

func (mq *Queue) waitSafeClose() {
	if mq.count.Load() == 0 {
		return
	}
	ticker := time.NewTicker(time.Second / 10)
	defer ticker.Stop()
	for range ticker.C {
		if mq.count.Load() == 0 {
			return
		}
	}
}
