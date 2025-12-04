package mq

import (
	"errors"
	"sync"
)

// Queue is a thread-safe queue implementation.
// It has a fixed capacity and supports adding tasks and getting the length of the queue.
// It also supports pausing and resuming the queue.

type Queue struct {
	cap   int
	head  int
	tail  int
	cond  *sync.Cond
	pause bool
	tasks []func()
}

func NewQueue(cap int) *Queue {
	if cap < 1 {
		panic("capacity must be greater than 0")
	}
	mq := &Queue{
		cap:   cap,
		cond:  sync.NewCond(&sync.Mutex{}),
		tasks: make([]func(), cap),
	}
	go mq.run()
	return mq
}

// Len returns the length of the queue.
func (mq *Queue) Len() int {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.len()
}

// Cap returns the capacity of the queue.
func (mq *Queue) Cap() int {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.cap
}

// Clear removes all tasks from the queue.
func (mq *Queue) Clear() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	if mq.len() > mq.cap/2 {
		mq.head = 0
		mq.tail = 0
		mq.tasks = make([]func(), mq.cap)
	} else {
		for i := mq.tail; i < mq.head; i++ {
			mq.tasks[i%mq.cap] = nil
		}
		mq.head = 0
		mq.tail = 0
	}
}

// Pause pauses the queue.
func (mq *Queue) Pause() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	mq.pause = true
}

// Resume resumes the queue.
func (mq *Queue) Resume() {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	mq.pause = false
	mq.cond.Signal()
}

// IsIdle returns true if the queue is empty.
func (mq *Queue) IsIdle() bool {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.len() == 0
}

// IsPaused returns true if the queue is paused.
func (mq *Queue) IsPaused() bool {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	return mq.pause
}

// AddTask adds a task to the queue.
func (mq *Queue) AddTask(task func()) error {
	mq.cond.L.Lock()
	defer mq.cond.L.Unlock()
	err := mq.enqueue(task)
	if err == nil {
		mq.cond.Signal()
	}
	return err
}
func (mq *Queue) run() {
	for {
		mq.cond.L.Lock()
		for mq.pause || mq.len() == 0 {
			mq.cond.Wait()
		}
		task := mq.dequeue()
		if mq.head >= mq.cap { // head pointer has wrapped around
			mq.head -= mq.cap
			mq.tail -= mq.cap
		}
		mq.cond.L.Unlock()
		task()
	}
}
func (mq *Queue) enqueue(task func()) error {
	if mq.len() == mq.cap {
		return errors.New("task array is full")
	}
	mq.tasks[mq.tail%mq.cap] = task
	mq.tail = (mq.tail + 1)
	return nil
}
func (mq *Queue) dequeue() func() {
	if mq.len() == 0 {
		return nil
	}
	task := mq.tasks[mq.head%mq.cap]
	mq.head = (mq.head + 1)
	return task
}
func (mq *Queue) len() int {
	return mq.tail - mq.head
}
