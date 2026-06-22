package services

import (
	"testing"
	"time"
)

func TestComputeDurationsMergesHeartbeatsWithinTimeoutByProject(t *testing.T) {
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "stint", Time: 1_700_000_000},
		{Entity: "b.go", Project: "stint", Time: 1_700_000_300},
		{Entity: "c.go", Project: "stint", Time: 1_700_001_500},
		{Entity: "x.go", Project: "other", Time: 1_700_000_100},
		{Entity: "y.go", Project: "other", Time: 1_700_000_400},
	}

	got := ComputeDurations(heartbeats, 15*time.Minute, "project")

	if len(got) != 5 {
		t.Fatalf("expected 5 duration rows, got %d: %#v", len(got), got)
	}
	if got[0].Name != "other" || got[0].DurationSeconds != 200 {
		t.Fatalf("expected first other span to be 200s, got %#v", got[0])
	}
	if got[1].Name != "other" || got[1].DurationSeconds != 0 {
		t.Fatalf("expected other span before the timeout gap to end the session at 0s, got %#v", got[1])
	}
	if got[2].Name != "stint" || got[2].DurationSeconds != 100 {
		t.Fatalf("expected first stint span to be 100s, got %#v", got[2])
	}
	if got[3].Name != "stint" || got[3].DurationSeconds != 100 {
		t.Fatalf("expected second stint span to be 100s, got %#v", got[3])
	}
	if got[4].Name != "stint" || got[4].DurationSeconds != 0 {
		t.Fatalf("expected final stint span to be 0s, got %#v", got[4])
	}
}

func TestComputeDurationsDoesNotDoubleCountOverlappingProjects(t *testing.T) {
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Time: 1_700_000_000},
		{Entity: "b.go", Project: "web", Time: 1_700_000_100},
		{Entity: "c.go", Project: "api", Time: 1_700_000_200},
		{Entity: "d.go", Project: "web", Time: 1_700_000_300},
	}

	got := ComputeDurations(heartbeats, 15*time.Minute, "project")
	total := 0
	for _, duration := range got {
		total += duration.DurationSeconds
	}

	if total != 300 {
		t.Fatalf("expected non-overlapping project durations to total 300s, got %d from %#v", total, got)
	}
}

func TestComputeDurationsEndsSessionAtLongGap(t *testing.T) {
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Time: 1_700_000_000},
		{Entity: "b.go", Project: "api", Time: 1_700_002_000},
	}

	got := ComputeDurations(heartbeats, 15*time.Minute, "project")

	// The gap (2000s) exceeds the 15m timeout, so it is idle time: the first
	// heartbeat ends a zero-length session and the second begins a new one.
	// WakaTime never credits time across a gap larger than the timeout.
	if len(got) != 2 {
		t.Fatalf("expected 2 duration rows, got %d: %#v", len(got), got)
	}
	if got[0].DurationSeconds != 0 {
		t.Fatalf("expected heartbeat before a long gap to contribute 0s, got %#v", got[0])
	}
	if got[1].DurationSeconds != 0 {
		t.Fatalf("expected final heartbeat to contribute 0s, got %#v", got[1])
	}
}

func TestComputeDurationsCanSliceByLanguage(t *testing.T) {
	heartbeats := []Heartbeat{
		{Entity: "a.go", Language: "Go", Time: 1_700_000_000},
		{Entity: "b.ts", Language: "TypeScript", Time: 1_700_000_100},
		{Entity: "c.go", Language: "Go", Time: 1_700_000_200},
	}

	got := ComputeDurations(heartbeats, 15*time.Minute, "language")

	if len(got) != 3 {
		t.Fatalf("expected 3 duration rows, got %d: %#v", len(got), got)
	}
	if got[0].Name != "Go" || got[0].DurationSeconds != 100 {
		t.Fatalf("expected Go duration to span 100s, got %#v", got[0])
	}
	if got[1].Name != "Go" || got[1].DurationSeconds != 0 {
		t.Fatalf("expected final Go span to be 0s, got %#v", got[1])
	}
	if got[2].Name != "TypeScript" || got[2].DurationSeconds != 100 {
		t.Fatalf("expected TypeScript duration to span 100s, got %#v", got[2])
	}
}

func TestComputeDurationsSplitsDependencyLists(t *testing.T) {
	heartbeats := []Heartbeat{
		{Entity: "a.go", Dependencies: "echo,pgx", Time: 1_700_000_000},
		{Entity: "b.go", Dependencies: "pgx, echo", Time: 1_700_000_120},
	}

	got := ComputeDurations(heartbeats, 15*time.Minute, "dependencies")

	if len(got) != 2 {
		t.Fatalf("expected one duration per dependency, got %d: %#v", len(got), got)
	}
	if got[0].Name != "echo" || got[0].DurationSeconds != 120 {
		t.Fatalf("expected echo duration to span 120s, got %#v", got[0])
	}
	if got[1].Name != "pgx" || got[1].DurationSeconds != 120 {
		t.Fatalf("expected pgx duration to span 120s, got %#v", got[1])
	}
}
