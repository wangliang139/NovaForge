package store

import (
	"context"
	"sync"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/strategy/infra/logging"
)

// BufferStorage is a logger that also keeps an in-memory cache of recent log entries.
type BufferStorage struct {
	max int

	mu   sync.RWMutex
	buf  []logging.Entry
	head int // oldest element index
	size int // number of valid elements

	timeFn func() time.Time
}

// BufferStorage creates a logger that caches the last maxEntries logs and forwards logs to next.
//
// - maxEntries <= 0: caching disabled (still forwards to next)
// - next == nil: logs are only cached (if enabled) and otherwise dropped
func NewBufferStorage(maxEntries int, timeFn func() time.Time) *BufferStorage {
	if maxEntries < 0 {
		maxEntries = 0
	}
	if timeFn == nil {
		timeFn = time.Now
	}
	return &BufferStorage{
		max:    maxEntries,
		timeFn: timeFn,
	}
}

func (l *BufferStorage) Max() int { return l.max }

func (l *BufferStorage) Count() int { return l.size }

func (l *BufferStorage) List() []logging.Entry {
	if l.max <= 0 {
		return nil
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.size == 0 {
		return nil
	}

	out := make([]logging.Entry, 0, l.size)
	for i := 0; i < l.size; i++ {
		idx := (l.head + i) % l.max
		out = append(out, l.buf[idx])
	}
	return out
}

func (l *BufferStorage) Write(ctx context.Context, entry logging.Entry) error {
	if l.max > 0 {
		l.mu.Lock()
		if l.buf == nil {
			l.buf = make([]logging.Entry, l.max)
		}

		// insert position = (head + size) % max; if full overwrite head and advance it
		pos := (l.head + l.size) % l.max
		l.buf[pos] = entry

		if l.size < l.max {
			l.size++
		} else {
			l.head = (l.head + 1) % l.max
		}
		l.mu.Unlock()
	}
	return nil
}
