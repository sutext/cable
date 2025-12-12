package queue

import (
	"sync"
	"sync/atomic"
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
	mu     sync.Mutex
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
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return int32(len(mq.tasks) + len(mq.jumps))
}
func (mq *Queue) IsIdle() bool {
	return mq.Len() == 0
}
func (mq *Queue) IsFull() bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()
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
func (mq *Queue) Push(task func()) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.closed.Load() {
		return ErrQueueIsStoped
	}
	select {
	case mq.tasks <- task:
		return nil
	default:
		return ErrQueueIsFull
	}
}

// Jump the queue to the front . If the queue is stop or empty, it will return ErrQueueIsStoped.
// Warning: Only one task is supported to jump the queue at the same time.
func (mq *Queue) Jump(task func()) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.closed.Load() {
		return ErrQueueIsStoped
	}
	mq.jumps <- task
	return nil
}
