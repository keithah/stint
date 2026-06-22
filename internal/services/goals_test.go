package services

import (
	"testing"
	"time"
)

func TestComputeGoalProgressForDailyGoalUsesTodayOnly(t *testing.T) {
	now := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "today.go", Project: "api", Language: "Go", Time: float64(now.Add(-2 * time.Hour).Unix())},
		{Entity: "today2.go", Project: "api", Language: "Go", Time: float64(now.Add(-2*time.Hour + 30*time.Minute).Unix())},
		{Entity: "yesterday.go", Project: "api", Language: "Go", Time: float64(now.AddDate(0, 0, -1).Unix())},
		{Entity: "yesterday2.go", Project: "api", Language: "Go", Time: float64(now.AddDate(0, 0, -1).Add(30 * time.Minute).Unix())},
	}
	goal := Goal{Title: "Code 1h", Delta: "day", Seconds: 3600, IsEnabled: true}

	got := ComputeGoalProgress(goal, heartbeats, now, 45*time.Minute)

	if got.ActualSeconds != 1800 {
		t.Fatalf("expected today-only actual seconds of 1800, got %d", got.ActualSeconds)
	}
	if got.Percent != 50 {
		t.Fatalf("expected 50 percent progress, got %d", got.Percent)
	}
	if got.IsComplete {
		t.Fatal("expected goal to be incomplete")
	}
	if got.RemainingSeconds != 1800 {
		t.Fatalf("expected 1800 seconds remaining, got %d", got.RemainingSeconds)
	}
}

func TestComputeGoalProgressFiltersByProjectAndLanguage(t *testing.T) {
	now := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Language: "Go", Time: float64(now.Add(-2 * time.Hour).Unix())},
		{Entity: "b.go", Project: "api", Language: "Go", Time: float64(now.Add(-2*time.Hour + 10*time.Minute).Unix())},
		{Entity: "page.tsx", Project: "web", Language: "TypeScript", Time: float64(now.Add(-1 * time.Hour).Unix())},
		{Entity: "component.tsx", Project: "web", Language: "TypeScript", Time: float64(now.Add(-1*time.Hour + 10*time.Minute).Unix())},
	}
	goal := Goal{Title: "Go API", Delta: "day", Seconds: 600, Projects: []string{"api"}, Languages: []string{"Go"}, IsEnabled: true}

	got := ComputeGoalProgress(goal, heartbeats, now, 15*time.Minute)

	if got.ActualSeconds != 600 {
		t.Fatalf("expected filtered actual seconds of 600, got %d", got.ActualSeconds)
	}
	if !got.IsComplete {
		t.Fatal("expected goal to be complete")
	}
}

func TestComputeGoalProgressFiltersByEditor(t *testing.T) {
	now := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Language: "Go", Editor: "vscode", Time: float64(now.Add(-2 * time.Hour).Unix())},
		{Entity: "b.go", Project: "api", Language: "Go", Editor: "vscode", Time: float64(now.Add(-2*time.Hour + 10*time.Minute).Unix())},
		{Entity: "c.go", Project: "api", Language: "Go", Editor: "vim", Time: float64(now.Add(-1 * time.Hour).Unix())},
		{Entity: "d.go", Project: "api", Language: "Go", Editor: "vim", Time: float64(now.Add(-1*time.Hour + 10*time.Minute).Unix())},
	}
	goal := Goal{Title: "VS Code", Delta: "day", Seconds: 600, Editors: []string{"vscode"}, IsEnabled: true}

	got := ComputeGoalProgress(goal, heartbeats, now, 15*time.Minute)

	if got.ActualSeconds != 600 {
		t.Fatalf("expected editor-filtered actual seconds of 600, got %d", got.ActualSeconds)
	}
	if !got.IsComplete {
		t.Fatal("expected editor-filtered goal to be complete")
	}
}

func TestComputeGoalProgressIncludesExternalDurations(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	goal := Goal{Seconds: 1800, Delta: "day", Projects: []string{"ops"}, Languages: []string{"Markdown"}}
	external := []ExternalDuration{
		{
			Provider:   "manual",
			ExternalID: "planning-1",
			Entity:     "Planning",
			Project:    "ops",
			Language:   "Markdown",
			StartTime:  float64(now.Add(-45 * time.Minute).Unix()),
			EndTime:    float64(now.Add(-15 * time.Minute).Unix()),
		},
	}

	got := ComputeGoalProgressWithExternalDurations(goal, nil, external, now, 15*time.Minute)

	if got.ActualSeconds != 1800 {
		t.Fatalf("expected external duration to count toward goal, got %d", got.ActualSeconds)
	}
	if !got.IsComplete {
		t.Fatalf("expected goal to be complete")
	}
}

func TestComputeGoalProgressIgnoresConfiguredWeekdays(t *testing.T) {
	friday := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Language: "Go", Time: float64(friday.Add(-2 * time.Hour).Unix())},
		{Entity: "b.go", Project: "api", Language: "Go", Time: float64(friday.Add(-2*time.Hour + 10*time.Minute).Unix())},
	}
	goal := Goal{Title: "No Fridays", Delta: "day", Seconds: 600, IgnoreDays: []string{"friday"}, IsEnabled: true}

	got := ComputeGoalProgress(goal, heartbeats, friday, 15*time.Minute)

	if got.ActualSeconds != 0 {
		t.Fatalf("expected ignored weekday to remove heartbeats, got %d seconds", got.ActualSeconds)
	}
	if got.IsComplete {
		t.Fatal("expected ignored weekday without ignore-zero-days to remain incomplete")
	}
}

func TestComputeGoalProgressIgnoreZeroDaysMarksGoalIgnored(t *testing.T) {
	now := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	goal := Goal{Title: "Only active days", Delta: "day", Seconds: 1800, IgnoreZeroDays: true, IsEnabled: true}

	got := ComputeGoalProgress(goal, nil, now, 15*time.Minute)

	if !got.IsIgnored {
		t.Fatal("expected zero-activity day to be marked ignored")
	}
	if !got.IsComplete {
		t.Fatal("expected ignored zero-activity day to be complete")
	}
	if got.Percent != 100 || got.RemainingSeconds != 0 {
		t.Fatalf("expected ignored goal to report complete progress, got percent=%d remaining=%d", got.Percent, got.RemainingSeconds)
	}
}

func TestComputeGoalProgressImprovesAgainstPreviousWindow(t *testing.T) {
	start := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	percent := 50.0
	heartbeats := []Heartbeat{
		{Entity: "prev.go", Project: "api", Language: "Go", Time: float64(start.AddDate(0, 0, -1).Add(9 * time.Hour).Unix())},
		{Entity: "prev2.go", Project: "api", Language: "Go", Time: float64(start.AddDate(0, 0, -1).Add(9*time.Hour + 20*time.Minute).Unix())},
		{Entity: "current.go", Project: "api", Language: "Go", Time: float64(start.Add(9 * time.Hour).Unix())},
		{Entity: "current2.go", Project: "api", Language: "Go", Time: float64(start.Add(9*time.Hour + 30*time.Minute).Unix())},
	}
	goal := Goal{Title: "Improve", Delta: "day", Seconds: 3600, ImproveByPercent: &percent, IsEnabled: true}

	got := ComputeGoalProgressForWindow(goal, heartbeats, start, end, 45*time.Minute)

	if got.TargetSeconds != 1800 {
		t.Fatalf("expected target to improve previous 1200 seconds by 50%%, got %d", got.TargetSeconds)
	}
	if got.ActualSeconds != 1800 {
		t.Fatalf("expected current actual seconds of 1800, got %d", got.ActualSeconds)
	}
	if !got.IsComplete {
		t.Fatal("expected improved goal to be complete")
	}
}

func TestComputeGoalProgressSnoozedGoal(t *testing.T) {
	now := time.Now().UTC()
	soon := now.Add(time.Hour).Format(time.RFC3339)
	goal := Goal{Title: "Snoozed", Delta: "day", Seconds: 1800, IsEnabled: true, IsSnoozed: true, SnoozeUntil: soon}
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Language: "Go", Time: float64(now.Add(-20 * time.Minute).Unix())},
		{Entity: "b.go", Project: "api", Language: "Go", Time: float64(now.Add(-10 * time.Minute).Unix())},
	}

	got := ComputeGoalProgress(goal, heartbeats, now, 15*time.Minute)

	if !got.IsSnoozed {
		t.Fatal("expected goal to be snoozed")
	}
	if got.ActualSeconds != 0 || got.Percent != 0 || got.IsComplete {
		t.Fatalf("expected snoozed goal to suppress progress, got actual=%d percent=%d complete=%v", got.ActualSeconds, got.Percent, got.IsComplete)
	}
}

func TestGoalEvaluationWindowUsesCompletedLocalDay(t *testing.T) {
	location := time.FixedZone("PST", -8*60*60)
	now := time.Date(2026, 6, 19, 0, 5, 0, 0, location)

	start, end := GoalEvaluationWindow("day", now)

	if want := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("expected start %s, got %s", want, start)
	}
	if want := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("expected end %s, got %s", want, end)
	}
}

func TestGoalEvaluationWindowUsesCompletedLocalWeek(t *testing.T) {
	now := time.Date(2026, 6, 22, 1, 0, 0, 0, time.UTC)

	start, end := GoalEvaluationWindow("week", now)

	if want := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("expected start %s, got %s", want, start)
	}
	if want := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("expected end %s, got %s", want, end)
	}
}

func TestComputeGoalProgressForWindowUsesExplicitBoundaries(t *testing.T) {
	start := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Language: "Go", Time: float64(start.Add(1 * time.Hour).Unix())},
		{Entity: "b.go", Project: "api", Language: "Go", Time: float64(start.Add(2 * time.Hour).Unix())},
		{Entity: "c.go", Project: "api", Language: "Go", Time: float64(end.Add(1 * time.Hour).Unix())},
	}
	goal := Goal{Title: "Code", Delta: "day", Seconds: 3600, IsEnabled: true}

	got := ComputeGoalProgressForWindow(goal, heartbeats, start, end, 2*time.Hour)

	if got.ActualSeconds != 3600 {
		t.Fatalf("expected one hour inside explicit window, got %d", got.ActualSeconds)
	}
	if !got.IsComplete {
		t.Fatal("expected goal to be complete")
	}
}

func TestComputeAllTimeStatsIncludesEveryHeartbeat(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "old.go", Project: "api", Language: "Go", Time: float64(now.AddDate(-2, 0, 0).Unix())},
		{Entity: "old2.go", Project: "api", Language: "Go", Time: float64(now.AddDate(-2, 0, 0).Add(10 * time.Minute).Unix())},
		{Entity: "new.ts", Project: "web", Language: "TypeScript", Time: float64(now.Add(-1 * time.Hour).Unix())},
		{Entity: "new2.ts", Project: "web", Language: "TypeScript", Time: float64(now.Add(-50 * time.Minute).Unix())},
	}

	got := ComputeAllTimeStats(heartbeats, 15*time.Minute)

	// api and web each span 10m within timeout; the ~2y gap between them is
	// idle and not credited. api=600s, web=600s, tie broken by name.
	if got.TotalSeconds != 1200 {
		t.Fatalf("expected 1200 all-time seconds, got %d", got.TotalSeconds)
	}
	if got.Projects[0].Name != "api" || got.Projects[0].TotalSeconds != 600 {
		t.Fatalf("expected api project first by name tie-break, got %#v", got.Projects)
	}
	if got.Range != "all_time" {
		t.Fatalf("expected all_time range, got %q", got.Range)
	}
}
