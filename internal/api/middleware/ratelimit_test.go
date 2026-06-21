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
