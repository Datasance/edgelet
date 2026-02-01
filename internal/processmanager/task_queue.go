package processmanager

import (
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

// Add adds a task to the queue
func (q *TaskQueue) Add(task *ContainerTask) {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	select {
	case q.tasks <- task:
		// Task added successfully
	default:
		// Queue is full - this shouldn't happen in normal operation
		// In production, we might want to log this or handle it differently
	}
}

// Get retrieves a task from the queue (blocks until available)
func (q *TaskQueue) Get() *ContainerTask {
	return <-q.tasks
}

// TryGet attempts to retrieve a task from the queue without blocking
func (q *TaskQueue) TryGet() (*ContainerTask, bool) {
	select {
	case task := <-q.tasks:
		return task, true
	default:
		return nil, false
	}
}

// Size returns the current number of tasks in the queue
func (q *TaskQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tasks)
}

// Close closes the task queue
func (q *TaskQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	close(q.tasks)
}
