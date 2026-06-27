package middleware

import (
	"context"
	"sync"
	"time"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error)
}

type MemoryRateLimiter struct {
	mu        sync.Mutex
	entries   map[string][]time.Time
	lastSweep time.Time
}

const memoryRateLimitSweepInterval = time.Minute

func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{entries: map[string][]time.Time{}}
}

func (l *MemoryRateLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if limit <= 0 {
		return true, 0, nil
	}
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) >= memoryRateLimitSweepInterval {
		l.sweepExpired(cutoff, now)
	}

	values := l.entries[key]
	active := values[:0]
	for _, value := range values {
		if value.After(cutoff) {
			active = append(active, value)
		}
	}
	if len(active) >= limit {
		l.entries[key] = active
		retryAfter := window - now.Sub(active[0])
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter, nil
	}
	active = append(active, now)
	l.entries[key] = active
	return true, 0, nil
}

func (l *MemoryRateLimiter) sweepExpired(cutoff, now time.Time) {
	for entryKey, values := range l.entries {
		active := values[:0]
		for _, value := range values {
			if value.After(cutoff) {
				active = append(active, value)
			}
		}
		if len(active) == 0 {
			delete(l.entries, entryKey)
			continue
		}
		l.entries[entryKey] = active
	}
	l.lastSweep = now
}
