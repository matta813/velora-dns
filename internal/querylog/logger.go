// Package querylog retains optional DNS history without blocking responses.
package querylog

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type Entry struct {
	ID         int64         `json:"id"`
	OccurredAt time.Time     `json:"occurred_at"`
	ClientIP   string        `json:"client_ip"`
	Domain     string        `json:"domain"`
	Type       string        `json:"type"`
	Rcode      string        `json:"rcode"`
	Source     string        `json:"source"`
	Upstream   string        `json:"upstream"`
	Duration   time.Duration `json:"duration"`
	CacheHit   bool          `json:"cache_hit"`
}
type Filter struct {
	Domain, Client, Type, Source string
	Limit                        int
	Before                       int64
}
type Store interface {
	WriteQueries(context.Context, []Entry, time.Time, int) error
}
type Stats struct {
	Written uint64 `json:"written"`
	Dropped uint64 `json:"dropped"`
	Errors  uint64 `json:"errors"`
}
type Logger struct {
	store                    Store
	queue                    chan Entry
	retention                time.Duration
	maxRows                  int
	enabled                  bool
	mu                       sync.RWMutex
	stopped                  bool
	written, dropped, errors atomic.Uint64
}

func New(store Store, enabled bool, capacity int, retention time.Duration, maxRows int) *Logger {
	return &Logger{store: store, enabled: enabled, queue: make(chan Entry, capacity), retention: retention, maxRows: maxRows}
}
func (l *Logger) Record(e Entry) {
	if !l.enabled {
		return
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.stopped {
		l.dropped.Add(1)
		return
	}
	select {
	case l.queue <- e:
	default:
		l.dropped.Add(1)
	}
}
func (l *Logger) Snapshot() Stats { return Stats{l.written.Load(), l.dropped.Load(), l.errors.Load()} }

// Run blocks until cancellation and drains accepted records with a bounded deadline.
// The application cancels this worker after DNS and HTTP listeners have stopped.
func (l *Logger) Run(ctx context.Context) {
	l.write(ctx, nil)
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			l.mu.Lock()
			l.stopped = true
			l.mu.Unlock()
			drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for len(l.queue) > 0 {
				l.write(drain, l.batch(<-l.queue))
			}
			return
		case e := <-l.queue:
			l.write(context.Background(), l.batch(e))
		case <-tick.C:
			l.write(ctx, nil)
		}
	}
}
func (l *Logger) batch(first Entry) []Entry {
	batch := []Entry{first}
	for len(batch) < 128 {
		select {
		case e := <-l.queue:
			batch = append(batch, e)
		default:
			return batch
		}
	}
	return batch
}
func (l *Logger) write(parent context.Context, entries []Entry) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := l.store.WriteQueries(ctx, entries, time.Now().Add(-l.retention), l.maxRows); err != nil {
		l.errors.Add(1)
		l.dropped.Add(uint64(len(entries)))
		return
	}
	l.written.Add(uint64(len(entries)))
}
