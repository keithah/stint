package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/keithah/stint/internal/services"
	"github.com/redis/go-redis/v9"
)

type StatusCache interface {
	Get(ctx context.Context, userID string) (services.StatusBarStats, bool, error)
	Set(ctx context.Context, userID string, status services.StatusBarStats, ttl time.Duration) error
}

type MemoryStatusCache struct {
	mu      sync.Mutex
	entries map[string]memoryStatusEntry
}

type memoryStatusEntry struct {
	status    services.StatusBarStats
	expiresAt time.Time
}

func NewMemoryStatusCache() *MemoryStatusCache {
	return &MemoryStatusCache{entries: map[string]memoryStatusEntry{}}
}

func (c *MemoryStatusCache) Get(_ context.Context, userID string) (services.StatusBarStats, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[userID]
	if !ok {
		return services.StatusBarStats{}, false, nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, userID)
		return services.StatusBarStats{}, false, nil
	}
	return entry.status, true, nil
}

func (c *MemoryStatusCache) Set(_ context.Context, userID string, status services.StatusBarStats, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[userID] = memoryStatusEntry{status: status, expiresAt: time.Now().Add(ttl)}
	return nil
}

type RedisStatusCache struct {
	client *redis.Client
}

func NewRedisStatusCache(redisURL string) (*RedisStatusCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisStatusCache{client: redis.NewClient(opts)}, nil
}

func (c *RedisStatusCache) Get(ctx context.Context, userID string) (services.StatusBarStats, bool, error) {
	raw, err := c.client.Get(ctx, statusKey(userID)).Bytes()
	if err == redis.Nil {
		return services.StatusBarStats{}, false, nil
	}
	if err != nil {
		return services.StatusBarStats{}, false, err
	}
	var status services.StatusBarStats
	if err := json.Unmarshal(raw, &status); err != nil {
		return services.StatusBarStats{}, false, err
	}
	return status, true, nil
}

func (c *RedisStatusCache) Set(ctx context.Context, userID string, status services.StatusBarStats, ttl time.Duration) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, statusKey(userID), raw, ttl).Err()
}

func statusKey(userID string) string {
	return "stint:status_bar:" + userID
}
