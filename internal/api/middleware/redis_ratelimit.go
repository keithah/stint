package middleware

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisRateLimitMemberCounter atomic.Uint64

var redisRateLimitScript = redis.NewScript(`
local key = KEYS[1]
local cutoff = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local window = tonumber(ARGV[4])
local member = ARGV[5]

redis.call("ZREMRANGEBYSCORE", key, "0", cutoff)
local count = redis.call("ZCARD", key)
if count >= limit then
  local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
  local retry_after = 1000
  if oldest[2] then
    retry_after = tonumber(oldest[2]) + window - now
    if retry_after < 1000 then
      retry_after = 1000
    end
  end
  return {0, retry_after}
end

redis.call("ZADD", key, now, member)
redis.call("PEXPIRE", key, window * 2)
return {1, 0}
`)

type RedisRateLimiter struct {
	client *redis.Client
}

func NewRedisRateLimiter(redisURL string) (*RedisRateLimiter, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisRateLimiter{client: redis.NewClient(opts)}, nil
}

func (l *RedisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if limit <= 0 {
		return true, 0, nil
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	cutoffMs := now.Add(-window).UnixMilli()
	redisKey := "stint:ratelimit:" + key

	member := fmt.Sprintf("%d:%d", now.UnixNano(), redisRateLimitMemberCounter.Add(1))
	result, err := redisRateLimitScript.Run(ctx, l.client, []string{redisKey}, cutoffMs, nowMs, limit, window.Milliseconds(), member).Int64Slice()
	if err != nil {
		return false, 0, err
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("unexpected Redis rate limit result: %v", result)
	}
	if result[0] == 0 {
		retryAfter := time.Duration(result[1]) * time.Millisecond
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter, nil
	}
	return true, 0, nil
}
