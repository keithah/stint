package middleware

import (
	"context"
	"testing"
	"time"
)

func TestMemoryRateLimiterBlocksWithinSlidingWindow(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	ctx := context.Background()
	window := 40 * time.Millisecond

	for i := 0; i < 2; i++ {
		allowed, _, err := limiter.Allow(ctx, "user:1", 2, window)
		if err != nil {
			t.Fatalf("Allow returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, retryAfter, err := limiter.Allow(ctx, "user:1", 2, window)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if allowed {
		t.Fatal("third request inside window should be blocked")
	}
	if retryAfter <= 0 {
		t.Fatal("expected positive retry-after duration")
	}

	time.Sleep(window + 10*time.Millisecond)
	allowed, _, err = limiter.Allow(ctx, "user:1", 2, window)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if !allowed {
		t.Fatal("request after window should be allowed")
	}
}

func TestMemoryRateLimiterSweepsExpiredKeysOnNewTraffic(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	ctx := context.Background()
	window := 20 * time.Millisecond

	if allowed, _, err := limiter.Allow(ctx, "ip:old", 1, window); err != nil || !allowed {
		t.Fatalf("old key should be allowed, allowed=%v err=%v", allowed, err)
	}
	limiter.lastSweep = time.Now().Add(-2 * memoryRateLimitSweepInterval)
	time.Sleep(window + 10*time.Millisecond)
	if allowed, _, err := limiter.Allow(ctx, "ip:new", 1, window); err != nil || !allowed {
		t.Fatalf("new key should be allowed, allowed=%v err=%v", allowed, err)
	}

	if _, ok := limiter.entries["ip:old"]; ok {
		t.Fatal("expected expired old key to be swept")
	}
}

func TestMemoryRateLimiterDoesNotSweepAllKeysOnEveryAllow(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	ctx := context.Background()
	window := 20 * time.Millisecond

	if allowed, _, err := limiter.Allow(ctx, "ip:old", 1, window); err != nil || !allowed {
		t.Fatalf("old key should be allowed, allowed=%v err=%v", allowed, err)
	}
	time.Sleep(window + 10*time.Millisecond)
	if allowed, _, err := limiter.Allow(ctx, "ip:new", 1, window); err != nil || !allowed {
		t.Fatalf("new key should be allowed, allowed=%v err=%v", allowed, err)
	}

	if _, ok := limiter.entries["ip:old"]; !ok {
		t.Fatal("expired unrelated key should not be swept on every request")
	}
}
