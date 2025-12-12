package poll

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

type Poll struct {
	mu    sync.Mutex
	tasks chan func()
}

// Create a new instance of Queue
// size is the maximum number of tasks that can be in the queue at any given time.
func New(length int32, workerCount int32) *Poll {
	if length < 1 || workerCount < 1 {
		panic("length and workerCount must be greater than 1")
	}
	mq := &Poll{
		tasks: make(chan func(), length),
	}
	for range workerCount {
		go func() {
			for task := range mq.tasks {
				task()
			}
		}()
	}
	return mq
}
func (mq *Poll) Len() int32 {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return int32(len(mq.tasks))
}
func (mq *Poll) IsIdle() bool {
	return mq.Len() == 0
}
func (mq *Poll) IsFull() bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.tasks) == cap(mq.tasks)
}
func (mq *Poll) IsClosed() bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return mq.tasks == nil
}

// Close is used to terminate the internal goroutines and close the channel.
func (mq *Poll) Close() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.tasks != nil {
		close(mq.tasks)
		mq.tasks = nil
	}
}

// Push a task to the queue. If the queue is full, it will return ErrQueueIsFull.
func (mq *Poll) Push(task func()) error {
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
