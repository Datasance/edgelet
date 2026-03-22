package processmanager

import (
	"context"
	"sync"
)

// TaskQueue manages container tasks in a thread-safe queue
type TaskQueue struct {
	tasks chan *ContainerTask
	mu    sync.RWMutex
}

// NewTaskQueue creates a new TaskQueue with the specified buffer size
func NewTaskQueue(bufferSize int) *TaskQueue {
	return &TaskQueue{
		tasks: make(chan *ContainerTask, bufferSize),
	}
}

// Add adds a task to the queue. Silently drops the task if the queue is full
// (capacity = 100); this should never happen under normal operating conditions.
func (q *TaskQueue) Add(task *ContainerTask) {
	q.mu.Lock()
	defer q.mu.Unlock()

	select {
	case q.tasks <- task:
	default:
		// Queue full — drop task rather than block.
	}
}

// Get blocks until a task is available or the context is canceled.
// Returns (task, true) on success and (nil, false) when the context is done.
func (q *TaskQueue) Get(ctx context.Context) (*ContainerTask, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case task := <-q.tasks:
		return task, true
	}
}

// TryGet attempts to retrieve a task from the queue without blocking.
func (q *TaskQueue) TryGet() (*ContainerTask, bool) {
	select {
	case task := <-q.tasks:
		return task, true
	default:
		return nil, false
	}
}

// Size returns the current number of tasks in the queue.
func (q *TaskQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks)
}

// Close closes the task queue channel.
func (q *TaskQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	close(q.tasks)
}
