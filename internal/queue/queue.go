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
	stoped atomic.Bool
}

// Create a new instance of Queue
// size is the maximum number of tasks that can be in the queue at any given time.
// workerCount is the number of worker goroutines to be created.
// If workerCount is not specified, it will default to 1.
func New(size int, workerCount ...int) *Queue {
	count := 1
	if len(workerCount) > 0 {
		count = workerCount[0]
	}
	if size < 1 || count < 1 {
		panic("size and workerCount must be greater than 0")
	}
	mq := &Queue{tasks: make(chan func(), size)}
	for range count {
		mq.wg.Go(func() {
			for task := range mq.tasks {
				task()
			}
		})
	}
	return mq
}

func (mq *Queue) Count() int64 {
	return mq.count.Load()
}

// Push a task to the queue. If the queue is full, it will return ErrQueueIsFull.
func (mq *Queue) Push(task func()) error {
	mq.count.Add(1)
	defer mq.count.Add(-1)
	if mq.stoped.Load() {
		return ErrQueueIsStoped
	}
	select {
	case mq.tasks <- task:
		return nil
	default:
		return ErrQueueIsFull
	}
}

// PushCtx is like Push, but it takes a context.Context as an argument.
// It will wait for the context to be done or for the task to be pushed to the queue.
// If the context is cancelled, it will return the error caused by the context.
func (mq *Queue) PushCtx(ctx context.Context, task func()) error {
	mq.count.Add(1)
	defer mq.count.Add(-1)
	if mq.stoped.Load() {
		return ErrQueueIsStoped
	}
	select {
	case mq.tasks <- task:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// Stop is used to terminate the internal goroutines and close the channel.
func (mq *Queue) Stop() {
	if !mq.stoped.CompareAndSwap(false, true) {
		return
	}
	mq.safeClose()
	close(mq.tasks)
	mq.wg.Wait()
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
