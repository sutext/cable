package queue

import (
	"context"
	"sync/atomic"
)

type Error uint8

var (
	ErrQueueIsFull   Error = 0
	ErrQueueIsClosed Error = 1
)

func (e Error) Error() string {
	switch e {
	case ErrQueueIsFull:
		return "queue is full"
	case ErrQueueIsClosed:
		return "queue is closed"
	default:
		return "unknown error"
	}
}

type Queue struct {
	jumps  chan func()
	tasks  chan func()
	closed atomic.Bool
}

// Create a new instance of Queue
// capacity is the maximum number of tasks that can be stored in the queue.
func New(capacity int32) *Queue {
	if capacity < 1 {
		panic("length and workerCount must be greater than 1")
	}
	mq := &Queue{
		tasks: make(chan func(), capacity),
		jumps: make(chan func(), 5),
	}
	go mq.run()
	return mq
}
func (mq *Queue) run() {
	for {
		select {
		case task, ok := <-mq.jumps:
			if !ok {
				return
			}
			task()
		default:
			select {
			case task, ok := <-mq.jumps:
				if !ok {
					return
				}
				task()
			case task, ok := <-mq.tasks:
				if !ok {
					return
				}
				task()
			}
		}
	}
}
func (mq *Queue) Len() int32 {
	return int32(len(mq.tasks) + len(mq.jumps))
}
func (mq *Queue) IsIdle() bool {
	return mq.Len() == 0
}
func (mq *Queue) IsFull() bool {
	return len(mq.tasks) == cap(mq.tasks)
}
func (mq *Queue) IsClosed() bool {
	return mq.closed.Load()
}

// Close is used to terminate the internal goroutines and close the channel.
// After calling Close, the queue cannot be used again.
func (mq *Queue) Close() {
	if mq.closed.CompareAndSwap(false, true) {
		close(mq.jumps) //close jumps channel to stop the queue
		mq.tasks = nil  //discard tasks channel to free resources
	}
}

// Push a task to the queue. If the queue is full, it will return ErrQueueIsFull.
// If the queue is closed, it will return ErrQueueIsStoped.
// If the context is canceled, it will return the context error.
// If the task is a jump task, it will be executed in jump queue.
func (mq *Queue) Push(ctx context.Context, jump bool, task func()) error {
	if mq.closed.Load() {
		return ErrQueueIsClosed
	}
	if jump {
		select {
		case mq.jumps <- task:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case mq.tasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
