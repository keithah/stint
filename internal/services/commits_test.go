package services

import (
	"testing"
	"time"
)

func TestComputeProjectCommitsAggregatesHeartbeatDurations(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Branch: "main", CommitHash: "abcdef1234567890", Time: float64(now.Add(-30 * time.Minute).Unix())},
		{Entity: "b.go", Project: "api", Branch: "main", CommitHash: "abcdef1234567890", Time: float64(now.Add(-20 * time.Minute).Unix())},
		{Entity: "c.go", Project: "api", Branch: "feature", CommitHash: "123456abcdef7890", Time: float64(now.Add(-10 * time.Minute).Unix())},
		{Entity: "d.go", Project: "api", Branch: "feature", CommitHash: "123456abcdef7890", Time: float64(now.Add(-5 * time.Minute).Unix())},
		{Entity: "web.tsx", Project: "web", Branch: "main", CommitHash: "ignored", Time: float64(now.Add(-5 * time.Minute).Unix())},
		{Entity: "missing.go", Project: "api", Branch: "main", Time: float64(now.Add(-4 * time.Minute).Unix())},
	}

	got := ComputeProjectCommits(heartbeats, "api", "", 15*time.Minute)

	if len(got) != 2 {
		t.Fatalf("expected two commits, got %#v", got)
	}
	if got[0].Hash != "123456abcdef7890" || got[0].TotalSeconds != 300 {
		t.Fatalf("expected newest feature commit first with 300s, got %#v", got[0])
	}
	if got[0].TruncatedHash != "123456a" || got[0].HumanReadableTotal != "5 mins" || got[0].HumanReadableTotalWithSeconds != "5 mins" {
		t.Fatalf("expected WakaTime-shaped display fields, got %#v", got[0])
	}
	if got[1].Hash != "abcdef1234567890" || got[1].TotalSeconds != 600 {
		t.Fatalf("expected main commit with 600s, got %#v", got[1])
	}
}

func TestComputeProjectCommitsFiltersByBranch(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Branch: "main", CommitHash: "mainhash", Time: float64(now.Add(-20 * time.Minute).Unix())},
		{Entity: "b.go", Project: "api", Branch: "main", CommitHash: "mainhash", Time: float64(now.Add(-10 * time.Minute).Unix())},
		{Entity: "c.go", Project: "api", Branch: "feature", CommitHash: "featurehash", Time: float64(now.Add(-5 * time.Minute).Unix())},
		{Entity: "d.go", Project: "api", Branch: "feature", CommitHash: "featurehash", Time: float64(now.Unix())},
	}

	got := ComputeProjectCommits(heartbeats, "api", "main", 15*time.Minute)

	if len(got) != 1 {
		t.Fatalf("expected one main branch commit, got %#v", got)
	}
	if got[0].Hash != "mainhash" || got[0].Branch != "main" {
		t.Fatalf("expected main hash, got %#v", got[0])
	}
}
