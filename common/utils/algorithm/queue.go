package algorithm

import (
	"fmt"
	"sync"
)

// Queue implements a FIFO Queue data structure.
type Queue[T comparable] struct {
	items []T
	mu    sync.RWMutex
}

// NewQueue creates a new FIFO queue where the items are stored in a plain slice.
func NewQueue[T comparable]() *Queue[T] {
	return &Queue[T]{
		mu: sync.RWMutex{},
	}
}

// Enqueue inserts a new element at the end of the queue.
func (q *Queue[T]) Enqueue(item T) {
	q.mu.Lock()
	q.items = append(q.items, item)
	q.mu.Unlock()
}

// Dequeue retrieves and removes the first element from the queue.
// The queue size will be decreased by one.
func (q *Queue[T]) Dequeue() (item T, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return item, fmt.Errorf("queue is empty")
	}

	item = q.items[0]
	q.items = q.items[1:]

	return
}

// Peek returns the first element of the queue without removing it.
func (q *Queue[T]) Peek() (item T) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.items) == 0 {
		return
	}

	return q.items[0]
}

// Search searches for an element in the queue.
func (q *Queue[T]) Search(item T) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for i := 0; i < len(q.items); i++ {
		if q.items[i] == item {
			return true
		}
	}

	return false
}

func (q *Queue[T]) ForEach(f func(T)) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for i := 0; i < len(q.items); i++ {
		f(q.items[i])
	}
}

// Size returns the FIFO queue size.
func (q *Queue[T]) Len() int {
	return q.Size()
}

func (q *Queue[T]) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.items)
}

// Clear erase all the items from the queue.
func (q *Queue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = nil
}
