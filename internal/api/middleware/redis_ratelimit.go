package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

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

	pipe := l.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, redisKey, "0", strconv.FormatInt(cutoffMs, 10))
	countCmd := pipe.ZCard(ctx, redisKey)
	oldestCmd := pipe.ZRangeWithScores(ctx, redisKey, 0, 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, err
	}
	if countCmd.Val() >= int64(limit) {
		retryAfter := time.Second
		if oldest := oldestCmd.Val(); len(oldest) > 0 {
			oldestMs := int64(oldest[0].Score)
			retryAfter = time.Duration(oldestMs+window.Milliseconds()-nowMs) * time.Millisecond
			if retryAfter < time.Second {
				retryAfter = time.Second
			}
		}
		return false, retryAfter, nil
	}

	member := fmt.Sprintf("%d:%d", now.UnixNano(), countCmd.Val())
	if err := l.client.ZAdd(ctx, redisKey, redis.Z{Score: float64(nowMs), Member: member}).Err(); err != nil {
		return false, 0, err
	}
	_ = l.client.Expire(ctx, redisKey, window*2).Err()
	return true, 0, nil
}
