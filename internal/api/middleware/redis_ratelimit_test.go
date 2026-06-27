package middleware

import (
	"os"
	"strings"
	"testing"
)

func TestRedisRateLimiterUsesAtomicLuaScript(t *testing.T) {
	source, err := os.ReadFile("redis_ratelimit.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "redis.NewScript") {
		t.Fatal("expected Redis rate limiter to use an atomic Lua script")
	}
	if strings.Contains(text, "TxPipeline") {
		t.Fatal("Redis rate limiter should not use a multi-command pipeline for sliding-window admission")
	}
}
