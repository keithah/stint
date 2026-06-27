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

const maxMemoryLeaderboardEntries = 128

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
	now := time.Now()
	c.sweepExpired(now)
	c.evictOldestIfFull()
	c.entries[rangeName] = memoryLeaderboardEntry{entries: append([]services.LeaderboardEntry(nil), entries...), expiresAt: now.Add(ttl)}
	return nil
}

func (c *MemoryLeaderboardCache) sweepExpired(now time.Time) {
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *MemoryLeaderboardCache) evictOldestIfFull() {
	if len(c.entries) < maxMemoryLeaderboardEntries {
		return
	}
	var oldestKey string
	var oldest time.Time
	for key, entry := range c.entries {
		if oldestKey == "" || entry.expiresAt.Before(oldest) {
			oldestKey = key
			oldest = entry.expiresAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
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

func (c *RedisLeaderboardCache) Close() error {
	return c.client.Close()
}

func leaderboardKey(rangeName string) string {
	return "stint:leaderboard:" + rangeName
}
