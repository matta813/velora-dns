// Package querylog writes optional DNS audit records without blocking DNS responses.
package querylog

import (
	"context"
	"sync"
	"time"
)

type Entry struct {
	OccurredAt                                      time.Time
	ClientIP, Domain, Type, Rcode, Source, Upstream string
	Duration                                        time.Duration
	CacheHit                                        bool
}
type Store interface {
	InsertQuery(context.Context, Entry) error
	PurgeQueries(context.Context, time.Time) error
}
type Logger struct {
	store     Store
	queue     chan Entry
	retention time.Duration
	enabled   bool
	once      sync.Once
}

func New(store Store, enabled bool, capacity int, retention time.Duration) *Logger {
	if capacity < 1 {
		capacity = 1
	}
	return &Logger{store: store, enabled: enabled, queue: make(chan Entry, capacity), retention: retention}
}
func (l *Logger) Record(e Entry) {
	if !l.enabled {
		return
	}
	select {
	case l.queue <- e:
	default:
	}
}
func (l *Logger) Run(ctx context.Context) {
	l.once.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case e := <-l.queue:
					_ = l.store.InsertQuery(ctx, e)
				case <-ticker.C:
					if l.retention > 0 {
						_ = l.store.PurgeQueries(ctx, time.Now().Add(-l.retention))
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	})
}
