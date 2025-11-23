package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type Error uint8

var (
	ErrQueueIsFull   Error = 0
	ErrQueueIsStoped Error = 1
)

func (e Error) Error() string {
	switch e {
	case ErrQueueIsStoped:
		return "queue is stopped"
	case ErrQueueIsFull:
		return "queue is full"
	default:
		return "unknown error"
	}
}

type Queue struct {
	wg     sync.WaitGroup
	count  atomic.Int64
	tasks  chan func()
	closed atomic.Bool
}

// Create a new instance of Queue
// size is the maximum number of tasks that can be in the queue at any given time.
// workerCount is the number of worker goroutines to be created.
// If workerCount is not specified, it will default to 1.
func New(length int, workerCount ...int) *Queue {
	count := 1
	if len(workerCount) > 0 {
		count = workerCount[0]
	}
	if length < 1 || count < 1 {
		panic("length and workerCount must be greater than 1")
	}
	mq := &Queue{tasks: make(chan func(), length)}
	for range count {
		mq.wg.Go(func() {
			for task := range mq.tasks {
				task()
				mq.count.Add(-1)
			}
		})
	}
	return mq
}
func (mq *Queue) IsIdle() bool {
	return mq.count.Load() == 0
}

// Close is used to terminate the internal goroutines and close the channel.
func (mq *Queue) Close() {
	if mq.closed.CompareAndSwap(false, true) {
		mq.safeClose()
		close(mq.tasks)
		mq.wg.Wait()
	}
}

// AddTask a task to the queue. If the queue is full, it will return ErrQueueIsFull.
func (mq *Queue) AddTask(task func()) error {
	if mq.closed.Load() {
		return ErrQueueIsStoped
	}
	mq.count.Add(1)
	select {
	case mq.tasks <- task:
		return nil
	default:
		mq.count.Add(-1)
		return ErrQueueIsFull
	}
}
func (mq *Queue) AddTaskCtx(ctx context.Context, task func()) error {
	if mq.closed.Load() {
		return ErrQueueIsStoped
	}
	mq.count.Add(1)
	select {
	case mq.tasks <- task:
		return nil
	case <-ctx.Done():
		mq.count.Add(-1)
		return ctx.Err()
	}
}
func (mq *Queue) safeClose() {
	if mq.count.Load() == 0 {
		return
	}
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for range ticker.C {
		if mq.count.Load() == 0 {
			return
		}
	}
}
