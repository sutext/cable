package queue

import (
	"sync"
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
	mu    sync.Mutex
	jumps chan func()
	tasks chan func()
}

// Create a new instance of Queue
// size is the maximum number of tasks that can be in the queue at any given time.
func New(length int) *Queue {
	if length < 1 {
		panic("length and workerCount must be greater than 1")
	}
	mq := &Queue{
		tasks: make(chan func()),
		jumps: make(chan func(), 3),
	}
	go mq.run()
	return mq
}
func (mq *Queue) run() {
	for {
		select {
		case task := <-mq.jumps:
			task()
		default:
			select {
			case task := <-mq.jumps:
				task()
			case task, ok := <-mq.tasks:
				if !ok {
					close(mq.jumps)
					mq.jumps = nil
					return
				}
				task()
			}
		}
	}
}
func (mq *Queue) Len() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.tasks) + len(mq.jumps)
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
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return mq.tasks == nil
}

// Close is used to terminate the internal goroutines and close the channel.
func (mq *Queue) Close() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.tasks != nil {
		close(mq.tasks)
		mq.tasks = nil
	}
}

// Push a task to the queue. If the queue is full, it will return ErrQueueIsFull.
func (mq *Queue) Push(task func()) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.tasks == nil {
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
	if mq.tasks == nil {
		return ErrQueueIsStoped
	}
	mq.jumps <- task
	return nil
}
