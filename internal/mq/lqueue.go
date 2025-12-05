package mq

import (
	"container/list"
	"errors"
	"sync"
)

// Queue is a thread-safe queue implementation.
// It has a fixed capacity and supports adding tasks and getting the length of the queue.
// It also supports pausing and resuming the queue.
type LQueue struct {
	cond   *sync.Cond
	tasks  *list.List
	paused bool
	closed bool
}

func NewLQueue() *LQueue {
	mq := &LQueue{
		cond:  sync.NewCond(&sync.Mutex{}),
		tasks: list.New(),
	}
	go mq.run()
	return mq
}

// Len returns the length of the queue.
func (mq *LQueue) Len() int {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.tasks.Len()
}

// Clear removes all tasks from the queue.
func (mq *LQueue) Clear() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	mq.tasks = list.New()
}

// Pause pauses the queue.
func (mq *LQueue) Pause() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	mq.paused = true
}

// Resume resumes the queue.
func (mq *LQueue) Resume() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	mq.paused = false
	mq.cond.Signal()
}
func (mq *LQueue) Close() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	mq.tasks.PushFront(func() bool { return true })
}

// IsIdle returns true if the queue is empty.
func (mq *LQueue) IsIdle() bool {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.tasks.Len() == 0
}

// IsPaused returns true if the queue is paused.
func (mq *LQueue) IsPaused() bool {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.paused
}
func (mq *LQueue) IsClosed() bool {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.closed
}

// AddTask adds a task to the queue.
func (mq *LQueue) AddTask(task func()) error {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	if mq.closed {
		return errors.New("queue is closed")
	}
	mq.tasks.PushBack(func() bool {
		task()
		return false
	})
	mq.cond.Signal()
	return nil
}
func (mq *LQueue) AddFirstTask(task func()) error {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	if mq.closed {
		return errors.New("queue is closed")
	}
	mq.tasks.PushFront(func() bool {
		task()
		return false
	})
	mq.cond.Signal()
	return nil
}
func (mq *LQueue) run() {
	for {
		mq.cond.L.Lock()
		for mq.paused || mq.tasks.Len() == 0 {
			mq.cond.Wait()
		}
		task := mq.tasks.Remove(mq.tasks.Front()).(func() bool)
		mq.cond.L.Unlock()
		stop := task()
		if stop {
			mq.cond.L.Lock()
			mq.closed = true
			mq.tasks = nil
			mq.cond.L.Unlock()
			break
		}
	}
}
