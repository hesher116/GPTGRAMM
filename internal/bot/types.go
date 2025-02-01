package bot

import (
	"sync"
	"time"
)

type UserSettings struct {
	Model        string
	RequestCount int
	LastRequest  time.Time
}

type MessageQueue struct {
	mu      sync.Mutex
	ids     []int
	maxSize int
}

func NewMessageQueue(size int) *MessageQueue {
	return &MessageQueue{
		ids:     make([]int, 0, size),
		maxSize: size,
	}
}

func (q *MessageQueue) Add(id int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ids = append(q.ids, id)
	if len(q.ids) > q.maxSize {
		q.ids = q.ids[1:]
	}
}

func (q *MessageQueue) GetAll() []int {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]int, len(q.ids))
	copy(result, q.ids)
	return result
}
