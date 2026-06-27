package cache

import (
	"context"
	"testing"
	"time"

	"github.com/keithah/stint/internal/services"
)

func TestMemoryStatusCacheExpiresEntries(t *testing.T) {
	cache := NewMemoryStatusCache()
	ctx := context.Background()
	want := services.StatusBarStats{TotalSeconds: 300, GrandTotalText: "5 mins", Range: "today"}

	if err := cache.Set(ctx, "user-1", want, 20*time.Millisecond); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	got, ok, err := cache.Get(ctx, "user-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected cached status")
	}
	if got.TotalSeconds != want.TotalSeconds || got.GrandTotalText != want.GrandTotalText {
		t.Fatalf("unexpected cached value: %#v", got)
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok, err := cache.Get(ctx, "user-1"); err != nil {
		t.Fatalf("Get after expiration returned error: %v", err)
	} else if ok {
		t.Fatal("expected cached status to expire")
	}
}

func TestMemoryLeaderboardCacheExpiresEntries(t *testing.T) {
	cache := NewMemoryLeaderboardCache()
	ctx := context.Background()
	want := []services.LeaderboardEntry{{Username: "alice", TotalSeconds: 600, Rank: 1}}

	if err := cache.Set(ctx, "last_7_days", want, 20*time.Millisecond); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	got, ok, err := cache.Get(ctx, "last_7_days")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected cached leaderboard")
	}
	if len(got) != 1 || got[0].Username != "alice" || got[0].TotalSeconds != 600 {
		t.Fatalf("unexpected cached leaderboard: %#v", got)
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok, err := cache.Get(ctx, "last_7_days"); err != nil {
		t.Fatalf("Get after expiration returned error: %v", err)
	} else if ok {
		t.Fatal("expected cached leaderboard to expire")
	}
}

func TestMemoryLeaderboardCacheCapsEntries(t *testing.T) {
	cache := NewMemoryLeaderboardCache()
	ctx := context.Background()

	for i := 0; i < maxMemoryLeaderboardEntries+10; i++ {
		key := "last_7_days:lang" + string(rune('a'+i))
		if err := cache.Set(ctx, key, []services.LeaderboardEntry{{Username: "alice"}}, time.Hour); err != nil {
			t.Fatalf("Set returned error: %v", err)
		}
	}

	if got := len(cache.entries); got > maxMemoryLeaderboardEntries {
		t.Fatalf("cache entries = %d, want at most %d", got, maxMemoryLeaderboardEntries)
	}
}

func TestMemoryStatusCacheCapsEntries(t *testing.T) {
	cache := NewMemoryStatusCache()
	ctx := context.Background()

	for i := 0; i < maxMemoryStatusEntries+10; i++ {
		key := "user-" + string(rune('a'+i))
		if err := cache.Set(ctx, key, services.StatusBarStats{TotalSeconds: i}, time.Hour); err != nil {
			t.Fatalf("Set returned error: %v", err)
		}
	}

	if got := len(cache.entries); got > maxMemoryStatusEntries {
		t.Fatalf("cache entries = %d, want at most %d", got, maxMemoryStatusEntries)
	}
}
