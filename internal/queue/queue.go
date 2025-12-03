package queue

import (
	"context"
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
	tasks chan func()
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
		go func() {
			for task := range mq.tasks {
				task()
			}
		}()
	}
	return mq
}

func (mq *Queue) IsIdle() bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.tasks) == 0
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

// AddTask a task to the queue. If the queue is full, it will return ErrQueueIsFull.
func (mq *Queue) AddTask(task func()) error {
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
func (mq *Queue) AddTaskCtx(ctx context.Context, task func()) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.tasks == nil {
		return ErrQueueIsStoped
	}
	select {
	case mq.tasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
