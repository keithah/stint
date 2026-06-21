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
	mu      sync.Mutex
	entries map[string][]time.Time
}

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
