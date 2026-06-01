package dispatcher

import (
	"log"
	"sync"

	"gitea-agent-gateway/internal/store"
)

// TaskQueue is an in-memory task queue backed by SQLite for persistence.
type TaskQueue struct {
	mu   sync.Mutex
	ch   chan *store.Task
	db   *store.DB
	size int
}

// NewTaskQueue creates a new task queue.
func NewTaskQueue(db *store.DB, size int) *TaskQueue {
	return &TaskQueue{
		ch:   make(chan *store.Task, size),
		db:   db,
		size: size,
	}
}

// Enqueue adds a task to the queue and persists it to SQLite.
func (q *TaskQueue) Enqueue(task *store.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Persist to DB
	if err := q.db.CreateTask(task); err != nil {
		return err
	}

	// Add to in-memory channel (non-blocking)
	select {
	case q.ch <- task:
		log.Printf("[INFO] Task enqueued: id=%d agent=%d type=%s", task.ID, task.AgentID, task.TaskType)
	default:
		log.Printf("[WARN] Task queue full, task %d will be picked up on restart", task.ID)
	}

	return nil
}

// Dequeue returns a channel that yields tasks.
func (q *TaskQueue) Dequeue() <-chan *store.Task {
	return q.ch
}

// LoadPending loads pending tasks from DB into the queue on startup.
func (q *TaskQueue) LoadPending() error {
	tasks, err := q.db.ListPendingTasks()
	if err != nil {
		return err
	}

	for _, t := range tasks {
		select {
		case q.ch <- t:
		default:
			log.Printf("[WARN] Queue full, skipping pending task %d", t.ID)
		}
	}

	log.Printf("[INFO] Loaded %d pending tasks from DB", len(tasks))
	return nil
}
