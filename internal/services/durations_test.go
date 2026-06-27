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

// TestComputeDurationsMatchesWakaTimeFAQExample encodes the worked example
// from WakaTime's own FAQ: "2 mins of coding, a 13 min break, then 1 min of
// coding" with a 15 min keystroke timeout totals 16 mins, because the 13 min
// break is shorter than the timeout and is filled in. A subsequent gap longer
// than the timeout is idle and adds nothing. This pins the WakaTime-parity
// intent so the cap-and-credit behavior cannot regress back in.
func TestComputeDurationsMatchesWakaTimeFAQExample(t *testing.T) {
	const base = 1_700_000_000.0
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "app", Time: base},              // start of 2 min burst
		{Entity: "a.go", Project: "app", Time: base + 120},        // ...end of 2 min burst
		{Entity: "a.go", Project: "app", Time: base + 900},        // 13 min break (< timeout, filled)
		{Entity: "a.go", Project: "app", Time: base + 960},        // ...end of trailing 1 min burst
		{Entity: "a.go", Project: "app", Time: base + 960 + 1200}, // 20 min idle (> timeout): new session
	}

	total := 0
	for _, d := range ComputeDurations(heartbeats, 15*time.Minute, "project") {
		total += d.DurationSeconds
	}

	// 120 (burst) + 780 (break) + 60 (burst) + 0 (idle > timeout) = 960s = 16 min.
	if total != 960 {
		t.Fatalf("expected WakaTime FAQ total of 960s (16 min), got %d", total)
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

func TestComputeDurationsCarriesWakaTimeAIFields(t *testing.T) {
	aiLines := 12
	humanLines := 3
	inputTokens := 1000
	outputTokens := 2000
	promptA := 120
	promptB := 80
	heartbeats := []Heartbeat{
		{
			Entity: "a.go", Project: "api", Time: 1_700_000_000, Category: "ai coding",
			AILineChanges: &aiLines, HumanLineChanges: &humanLines, AIInputTokens: &inputTokens, AIOutputTokens: &outputTokens,
			AIPromptLength: &promptA, AISession: "session-a", AISubscriptionPlan: "Codex",
		},
		{
			Entity: "b.go", Project: "api", Time: 1_700_000_120, Category: "ai coding",
			AIPromptLength: &promptB, AISession: "session-a", AISubscriptionPlan: "Codex",
		},
	}

	got := ComputeDurations(heartbeats, 15*time.Minute, "project")

	if len(got) != 1 {
		t.Fatalf("expected merged AI duration row, got %#v", got)
	}
	row := got[0]
	if row.DurationSeconds != 120 {
		t.Fatalf("expected merged duration of 120s, got %#v", row)
	}
	if row.AIAdditions != 12 || row.AIDeletions != 0 || row.HumanAdditions != 3 || row.HumanDeletions != 0 {
		t.Fatalf("expected WakaTime duration line aliases, got %#v", row)
	}
	if row.AIInputTokens != 1000 || row.AIOutputTokens != 2000 {
		t.Fatalf("expected duration token totals, got %#v", row)
	}
	if row.AIPromptLengthSum != 200 || row.AIPromptLengthAvg != 100 {
		t.Fatalf("expected duration prompt totals, got %#v", row)
	}
	if row.AISessions != 1 || row.AIPromptEventsTotal != 2 || row.AIPromptEventsAvgPerSession != 2 || row.AIPromptLengthAvgPerSession != 200 {
		t.Fatalf("expected duration prompt session stats, got %#v", row)
	}
	if row.AIAgentCosts["Codex"] != 0 {
		t.Fatalf("expected duration agent cost map to include Codex with zero cost when rates are unavailable, got %#v", row.AIAgentCosts)
	}
}
