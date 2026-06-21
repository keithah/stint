package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/keithah/stint/internal/services"
	"github.com/redis/go-redis/v9"
)

type LeaderboardCache interface {
	Get(ctx context.Context, rangeName string) ([]services.LeaderboardEntry, bool, error)
	Set(ctx context.Context, rangeName string, entries []services.LeaderboardEntry, ttl time.Duration) error
}

type MemoryLeaderboardCache struct {
	mu      sync.Mutex
	entries map[string]memoryLeaderboardEntry
}

type memoryLeaderboardEntry struct {
	entries   []services.LeaderboardEntry
	expiresAt time.Time
}

func NewMemoryLeaderboardCache() *MemoryLeaderboardCache {
	return &MemoryLeaderboardCache{entries: map[string]memoryLeaderboardEntry{}}
}

func (c *MemoryLeaderboardCache) Get(_ context.Context, rangeName string) ([]services.LeaderboardEntry, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[rangeName]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, rangeName)
		return nil, false, nil
	}
	return append([]services.LeaderboardEntry(nil), entry.entries...), true, nil
}

func (c *MemoryLeaderboardCache) Set(_ context.Context, rangeName string, entries []services.LeaderboardEntry, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[rangeName] = memoryLeaderboardEntry{entries: append([]services.LeaderboardEntry(nil), entries...), expiresAt: time.Now().Add(ttl)}
	return nil
}

type RedisLeaderboardCache struct {
	client *redis.Client
}

func NewRedisLeaderboardCache(redisURL string) (*RedisLeaderboardCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisLeaderboardCache{client: redis.NewClient(opts)}, nil
}

func (c *RedisLeaderboardCache) Get(ctx context.Context, rangeName string) ([]services.LeaderboardEntry, bool, error) {
	raw, err := c.client.Get(ctx, leaderboardKey(rangeName)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var entries []services.LeaderboardEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

func (c *RedisLeaderboardCache) Set(ctx context.Context, rangeName string, entries []services.LeaderboardEntry, ttl time.Duration) error {
	raw, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, leaderboardKey(rangeName), raw, ttl).Err()
}

func leaderboardKey(rangeName string) string {
	return "stint:leaderboard:" + rangeName
}
