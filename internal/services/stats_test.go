package services

import (
	"testing"
	"time"
)

func TestComputeLast7DaysStatsAggregatesTotalsProjectsAndLanguages(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "stint", Language: "Go", Time: float64(now.Add(-24 * time.Hour).Unix())},
		{Entity: "b.go", Project: "stint", Language: "Go", Time: float64(now.Add(-24*time.Hour + 10*time.Minute).Unix())},
		{Entity: "app.tsx", Project: "web", Language: "TypeScript", Time: float64(now.Add(-48 * time.Hour).Unix())},
		{Entity: "page.tsx", Project: "web", Language: "TypeScript", Time: float64(now.Add(-48*time.Hour + 5*time.Minute).Unix())},
		{Entity: "old.go", Project: "old", Language: "Go", Time: float64(now.Add(-9 * 24 * time.Hour).Unix())},
	}

	got := ComputeLast7DaysStats(heartbeats, now, 15*time.Minute)

	// Each project's two heartbeats are 10m/5m apart (within timeout); the
	// ~24h gap between the two days exceeds the timeout, so no idle time is
	// credited across it. stint=600s, web=300s.
	if got.TotalSeconds != 900 {
		t.Fatalf("expected 900 total seconds, got %d", got.TotalSeconds)
	}
	if got.DailyAverageSeconds != 128 {
		t.Fatalf("expected rounded-down daily average of 128s, got %d", got.DailyAverageSeconds)
	}
	if got.BestDay.Date != "2026-06-18" || got.BestDay.TotalSeconds != 600 {
		t.Fatalf("expected 2026-06-18 as best day with 600s, got %#v", got.BestDay)
	}
	if got.Projects[0].Name != "stint" || got.Projects[0].TotalSeconds != 600 {
		t.Fatalf("expected stint project with 600s first, got %#v", got.Projects)
	}
	if got.Languages[0].Name != "Go" || got.Languages[0].TotalSeconds != 600 {
		t.Fatalf("expected Go language with 600s first, got %#v", got.Languages)
	}
	if len(got.Days) != 7 {
		t.Fatalf("expected 7 daily buckets, got %d", len(got.Days))
	}
	var stintDay DailyStat
	for _, day := range got.Days {
		if day.Date == "2026-06-18" {
			stintDay = day
			break
		}
	}
	if len(stintDay.Projects) != 1 || stintDay.Projects[0].Name != "stint" || stintDay.Projects[0].TotalSeconds != 600 {
		t.Fatalf("expected daily project slices for stint day, got %#v", stintDay.Projects)
	}
}

func TestComputeStatsForRangeUsesNowLocationForDailyBuckets(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, location)
	first := time.Date(2026, 6, 20, 23, 30, 0, 0, location)
	second := first.Add(10 * time.Minute)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "stint", Time: float64(first.Unix())},
		{Entity: "b.go", Project: "stint", Time: float64(second.Unix())},
	}

	got, window, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if window.Start.Location() != location {
		t.Fatalf("expected window to use LA location, got %v", window.Start.Location())
	}
	for _, day := range got.Days {
		if day.Date == "2026-06-20" && day.TotalSeconds != 600 {
			t.Fatalf("expected LA June 20 to contain 600s, got %#v", day)
		}
		if day.Date == "2026-06-21" && day.TotalSeconds != 0 {
			t.Fatalf("expected LA June 21 to stay empty, got %#v", day)
		}
	}
}

func TestComputeStatsForRangeMapsMissingLanguageToOther(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "/tmp/project", Type: "app", Project: "project", Time: float64(now.Add(-10 * time.Minute).Unix())},
		{Entity: "/tmp/project/main.go", Type: "file", Project: "project", Language: "Go", Time: float64(now.Unix())},
	}

	got, _, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Languages) == 0 || got.Languages[0].Name != "Other" {
		t.Fatalf("expected missing language to be grouped as Other, got %#v", got.Languages)
	}
	for _, language := range got.Languages {
		if language.Name == "Unknown" {
			t.Fatalf("expected no Unknown language bucket, got %#v", got.Languages)
		}
	}
}

func TestComputeStatsForRangeOmitsMissingBranch(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "/tmp/project", Type: "app", Project: "project", Time: float64(now.Add(-10 * time.Minute).Unix())},
		{Entity: "/tmp/project/main.go", Type: "file", Project: "project", Branch: "main", Time: float64(now.Unix())},
	}

	got, _, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Branches) != 1 || got.Branches[0].Name != "main" {
		t.Fatalf("expected only concrete branch buckets, got %#v", got.Branches)
	}
}

func TestComputeStatsForRangeSupportsLast30DaysAndExtraBreakdowns(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "stint", Branch: "main", Language: "Go", Editor: "vscode", OperatingSystem: "linux", MachineName: "laptop", Category: "coding", Dependencies: "pgx", Time: float64(now.AddDate(0, 0, -29).Unix())},
		{Entity: "b.go", Project: "stint", Branch: "main", Language: "Go", Editor: "vscode", OperatingSystem: "linux", MachineName: "laptop", Category: "coding", Dependencies: "pgx", Time: float64(now.AddDate(0, 0, -29).Add(10 * time.Minute).Unix())},
		{Entity: "page.tsx", Project: "web", Language: "TypeScript", Editor: "zed", OperatingSystem: "darwin", MachineName: "desktop", Category: "debugging", Time: float64(now.AddDate(0, 0, -31).Unix())},
		{Entity: "old.tsx", Project: "web", Language: "TypeScript", Editor: "zed", OperatingSystem: "darwin", MachineName: "desktop", Category: "debugging", Time: float64(now.AddDate(0, 0, -31).Add(10 * time.Minute).Unix())},
	}

	got, window, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_30_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if window.Days != 30 {
		t.Fatalf("expected a 30 day window, got %d", window.Days)
	}
	if got.Range != "last_30_days" {
		t.Fatalf("expected range last_30_days, got %q", got.Range)
	}
	if got.TotalSeconds != 600 {
		t.Fatalf("expected only in-range duration to count, got %d", got.TotalSeconds)
	}
	if len(got.Days) != 30 {
		t.Fatalf("expected 30 daily buckets, got %d", len(got.Days))
	}
	if got.Editors[0].Name != "vscode" || got.Editors[0].TotalSeconds != 600 {
		t.Fatalf("expected editor totals, got %#v", got.Editors)
	}
	if got.OperatingSystems[0].Name != "linux" || got.OperatingSystems[0].TotalSeconds != 600 {
		t.Fatalf("expected OS totals, got %#v", got.OperatingSystems)
	}
	if got.Machines[0].Name != "laptop" || got.Machines[0].TotalSeconds != 600 {
		t.Fatalf("expected machine totals, got %#v", got.Machines)
	}
	if got.Categories[0].Name != "coding" || got.Categories[0].TotalSeconds != 600 {
		t.Fatalf("expected category totals, got %#v", got.Categories)
	}
	if got.Branches[0].Name != "main" || got.Branches[0].TotalSeconds != 600 {
		t.Fatalf("expected branch totals, got %#v", got.Branches)
	}
	if got.Dependencies[0].Name != "pgx" || got.Dependencies[0].TotalSeconds != 600 {
		t.Fatalf("expected dependency totals, got %#v", got.Dependencies)
	}
}

func TestComputeStatsForRangeSupportsCalendarYearRange(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "jan-a.go", Project: "api", Language: "Go", Time: float64(time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC).Unix())},
		{Entity: "jan-b.go", Project: "api", Language: "Go", Time: float64(time.Date(2026, 1, 5, 10, 10, 0, 0, time.UTC).Unix())},
		{Entity: "old.go", Project: "old", Language: "Go", Time: float64(time.Date(2025, 12, 31, 23, 55, 0, 0, time.UTC).Unix())},
	}

	got, window, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if window.Range != "2026" || !window.Start.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !window.End.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected calendar year window: %#v", window)
	}
	if window.Days != 365 {
		t.Fatalf("expected 365 day year window, got %d", window.Days)
	}
	if got.TotalSeconds != 600 {
		t.Fatalf("expected only 2026 duration to count, got %d", got.TotalSeconds)
	}
}

func TestComputeStatsForRangeSupportsCalendarMonthRange(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "jun-a.go", Project: "api", Language: "Go", Time: float64(time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC).Unix())},
		{Entity: "jun-b.go", Project: "api", Language: "Go", Time: float64(time.Date(2026, 6, 5, 10, 10, 0, 0, time.UTC).Unix())},
		{Entity: "jul.go", Project: "next", Language: "Go", Time: float64(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Unix())},
	}

	got, window, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "2026-06")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if window.Range != "2026-06" || !window.Start.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) || !window.End.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected calendar month window: %#v", window)
	}
	if window.Days != 30 {
		t.Fatalf("expected 30 day month window, got %d", window.Days)
	}
	if got.TotalSeconds != 600 {
		t.Fatalf("expected only June duration to count, got %d", got.TotalSeconds)
	}
}

func TestComputeStatsForRangeIncludesExternalDurations(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -1).Truncate(time.Hour)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Branch: "main", Language: "Go", Editor: "vscode", OperatingSystem: "linux", MachineName: "laptop", Category: "coding", Time: float64(start.Unix())},
		{Entity: "b.go", Project: "api", Branch: "main", Language: "Go", Editor: "vscode", OperatingSystem: "linux", MachineName: "laptop", Category: "coding", Time: float64(start.Add(10 * time.Minute).Unix())},
	}
	external := []ExternalDuration{
		{
			Provider:   "manual",
			ExternalID: "planning-1",
			Entity:     "Planning",
			Type:       "app",
			Category:   "planning",
			Project:    "ops",
			Branch:     "roadmap",
			Language:   "Markdown",
			StartTime:  float64(start.Add(1 * time.Hour).Unix()),
			EndTime:    float64(start.Add(1*time.Hour + 30*time.Minute).Unix()),
		},
	}

	got, _, err := ComputeStatsForRangeWithExternalDurations(heartbeats, external, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.TotalSeconds != 2400 {
		t.Fatalf("expected heartbeat and external totals, got %d", got.TotalSeconds)
	}
	if got.Projects[0].Name != "ops" || got.Projects[0].TotalSeconds != 1800 {
		t.Fatalf("expected external project total first, got %#v", got.Projects)
	}
	if !hasSliceTotal(got.Languages, "Markdown", 1800) {
		t.Fatalf("expected external language total, got %#v", got.Languages)
	}
	if !hasSliceTotal(got.Categories, "planning", 1800) {
		t.Fatalf("expected external category total, got %#v", got.Categories)
	}
	if !hasSliceTotal(got.Branches, "roadmap", 1800) {
		t.Fatalf("expected external branch total, got %#v", got.Branches)
	}
	if len(got.Editors) != 1 || got.Editors[0].Name != "vscode" || got.Editors[0].TotalSeconds != 600 {
		t.Fatalf("expected editor totals to remain heartbeat-derived, got %#v", got.Editors)
	}
	day := start.Format("2006-01-02")
	var dailyTotal int
	for _, row := range got.Days {
		if row.Date == day {
			dailyTotal = row.TotalSeconds
			break
		}
	}
	if dailyTotal != 2400 {
		t.Fatalf("expected daily total with external duration, got %d", dailyTotal)
	}
	if got.Hourly[start.Add(time.Hour).Hour()].TotalSeconds != 1800 {
		t.Fatalf("expected external duration in hourly timeline, got %#v", got.Hourly[start.Add(time.Hour).Hour()])
	}
}

func TestComputeStatusBarTodayReturnsCurrentProjectAndLanguage(t *testing.T) {
	now := time.Date(2026, 6, 19, 18, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "a.go", Project: "api", Language: "Go", Time: float64(now.Add(-2 * time.Hour).Unix())},
		{Entity: "b.go", Project: "api", Language: "Go", Time: float64(now.Add(-2*time.Hour + 5*time.Minute).Unix())},
		{Entity: "page.tsx", Project: "web", Language: "TypeScript", Time: float64(now.Add(-1 * time.Hour).Unix())},
		{Entity: "component.tsx", Project: "web", Language: "TypeScript", Time: float64(now.Add(-1*time.Hour + 10*time.Minute).Unix())},
	}

	got := ComputeStatusBarToday(heartbeats, now, 15*time.Minute)

	// api spans 5m, web spans 10m; the ~55m gap between them exceeds the
	// timeout and is not credited. api=300s, web=600s.
	if got.TotalSeconds != 900 {
		t.Fatalf("expected 900 total seconds, got %d", got.TotalSeconds)
	}
	if got.Project != "web" || got.ProjectSeconds != 600 {
		t.Fatalf("expected active project web with 600 seconds, got %#v", got)
	}
	if got.Language != "TypeScript" || got.LanguageSeconds != 600 {
		t.Fatalf("expected active language TypeScript with 600 seconds, got %#v", got)
	}
}

func TestComputeStatusBarForWindowUsesExplicitLocalDayAcrossUTCMidnight(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	start := time.Date(2026, 6, 18, 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	heartbeats := []Heartbeat{
		{Entity: "before-midnight-utc.go", Project: "api", Language: "Go", Time: float64(time.Date(2026, 6, 18, 23, 50, 0, 0, time.UTC).Unix())},
		{Entity: "after-midnight-utc.go", Project: "api", Language: "Go", Time: float64(time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC).Unix())},
	}
	external := []ExternalDuration{{
		Provider:   "manual",
		ExternalID: "planning",
		Entity:     "Planning",
		Type:       "app",
		Project:    "ops",
		Language:   "Markdown",
		StartTime:  float64(time.Date(2026, 6, 18, 22, 0, 0, 0, time.UTC).Unix()),
		EndTime:    float64(time.Date(2026, 6, 18, 22, 30, 0, 0, time.UTC).Unix()),
	}}

	got := ComputeStatusBarForWindowWithExternalDurations(heartbeats, external, start, end, 15*time.Minute)

	if got.TotalSeconds != 2400 {
		t.Fatalf("expected local-day heartbeat and external totals, got %d", got.TotalSeconds)
	}
	if got.Project != "ops" || got.ProjectSeconds != 1800 {
		t.Fatalf("expected top project from local-day external duration, got %#v", got)
	}
	if got.Language != "Markdown" || got.LanguageSeconds != 1800 {
		t.Fatalf("expected top language from local-day external duration, got %#v", got)
	}
}

func hasSliceTotal(rows []SliceTotal, name string, total int) bool {
	for _, row := range rows {
		if row.Name == name && row.TotalSeconds == total {
			return true
		}
	}
	return false
}

func TestComputeStatsForRangeAggregatesAIMetrics(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	aiLinesA := 80
	aiLinesB := 20
	humanLines := 50
	inputA := 1000
	outputA := 500
	inputB := 2000
	outputB := 1000
	promptA := 200
	promptB := 300
	heartbeats := []Heartbeat{
		{
			Entity: "agent.go", Project: "api", Language: "Go", Time: float64(now.Add(-2 * time.Hour).Unix()),
			AILineChanges: &aiLinesA, HumanLineChanges: &humanLines, AISession: "session-a",
			AIInputTokens: &inputA, AIOutputTokens: &outputA, AIPromptLength: &promptA, AISubscriptionPlan: "Codex",
		},
		{
			Entity: "agent2.go", Project: "api", Language: "Go", Time: float64(now.Add(-2*time.Hour + 10*time.Minute).Unix()),
			AILineChanges: &aiLinesB, AISession: "session-b",
			AIInputTokens: &inputB, AIOutputTokens: &outputB, AIPromptLength: &promptB, AISubscriptionPlan: "Codex",
		},
	}

	got, _, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.AI.AILineChanges != 100 {
		t.Fatalf("expected 100 AI line changes, got %d", got.AI.AILineChanges)
	}
	if got.AI.AIAdditions != 100 || got.AI.AIDeletions != 0 || got.AI.AILineChangesTotal != 100 {
		t.Fatalf("expected WakaTime AI line aliases to mirror line changes, got %#v", got.AI)
	}
	if got.AI.HumanLineChanges != 50 {
		t.Fatalf("expected 50 human line changes, got %d", got.AI.HumanLineChanges)
	}
	if got.AI.HumanAdditions != 50 || got.AI.HumanDeletions != 0 {
		t.Fatalf("expected WakaTime human line aliases to mirror line changes, got %#v", got.AI)
	}
	if got.AI.AIPercentage != 66 {
		t.Fatalf("expected 66 AI percentage, got %d", got.AI.AIPercentage)
	}
	if got.AI.AIInputTokens != 3000 || got.AI.AIOutputTokens != 1500 || got.AI.AIPromptLength != 500 {
		t.Fatalf("unexpected token or prompt totals: %#v", got.AI)
	}
	if got.AI.SessionCount != 2 {
		t.Fatalf("expected 2 sessions, got %d", got.AI.SessionCount)
	}
	if got.AI.PromptCount != 2 {
		t.Fatalf("expected 2 prompts, got %d", got.AI.PromptCount)
	}
	if got.AI.AISessions != 2 {
		t.Fatalf("expected WakaTime ai_sessions alias to be 2, got %d", got.AI.AISessions)
	}
	if got.AI.AveragePromptLength != 250 {
		t.Fatalf("expected 250 average prompt length, got %d", got.AI.AveragePromptLength)
	}
	if got.AI.AIPromptLengthAvg != 250 || got.AI.AIPromptLengthSum != 500 {
		t.Fatalf("expected WakaTime prompt length aliases, got %#v", got.AI)
	}
	if got.AI.MedianPromptLength != 250 {
		t.Fatalf("expected 250 median prompt length, got %d", got.AI.MedianPromptLength)
	}
	if got.AI.AIPromptLengthAvgPerSession != 250 || got.AI.AIPromptLengthMedianPerSession != 250 {
		t.Fatalf("expected WakaTime per-session prompt length stats, got %#v", got.AI)
	}
	if got.AI.AIPromptEventsTotal != 2 || got.AI.AIPromptEventsAvgPerSession != 1 || got.AI.AIPromptEventsMedianPerSession != 1 {
		t.Fatalf("expected WakaTime per-session prompt event stats, got %#v", got.AI)
	}
	if got.AI.FollowUpEdits != 50 {
		t.Fatalf("expected 50 follow-up edits, got %d", got.AI.FollowUpEdits)
	}
	if got.AI.HumanReviewPercentage != 50 {
		t.Fatalf("expected 50 human review percentage, got %d", got.AI.HumanReviewPercentage)
	}
	if len(got.AI.Agents) != 1 || got.AI.Agents[0].Name != "Codex" || got.AI.Agents[0].AILineChanges != 100 {
		t.Fatalf("expected Codex agent totals, got %#v", got.AI.Agents)
	}
	if got.AI.AIAgentLineChanges["Codex"] != 100 {
		t.Fatalf("expected WakaTime agent line-change map, got %#v", got.AI.AIAgentLineChanges)
	}
	if len(got.AI.AIAgentBreakdown) != 1 || got.AI.AIAgentBreakdown[0].Name != "Codex" || got.AI.AIAgentBreakdown[0].Lines != 100 {
		t.Fatalf("expected WakaTime agent breakdown, got %#v", got.AI.AIAgentBreakdown)
	}
	if len(got.AI.Days) == 0 || got.AI.Days[len(got.AI.Days)-1].AILineChanges != 100 {
		t.Fatalf("expected AI day totals on latest bucket, got %#v", got.AI.Days)
	}
}

func TestComputeStatsForRangeCountsAICodingCategoryAsAISeconds(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeats := []Heartbeat{
		{Entity: "agent.go", Project: "api", Category: "ai coding", Time: float64(now.Add(-2 * time.Hour).Unix())},
		{Entity: "agent2.go", Project: "api", Category: "ai coding", Time: float64(now.Add(-2*time.Hour + 10*time.Minute).Unix())},
		{Entity: "human.go", Project: "api", Category: "coding", Time: float64(now.Add(-time.Hour).Unix())},
		{Entity: "human2.go", Project: "api", Category: "coding", Time: float64(now.Add(-time.Hour + 10*time.Minute).Unix())},
	}

	got, _, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.AI.Days[len(got.AI.Days)-1].AISeconds != 600 {
		t.Fatalf("expected only ai coding duration to count as AI seconds, got %#v", got.AI.Days[len(got.AI.Days)-1])
	}
	if len(got.ProjectAI) != 1 || got.ProjectAI[0].AISeconds != 600 {
		t.Fatalf("expected project AI duration from ai coding category only, got %#v", got.ProjectAI)
	}
}

func TestComputeStatsForRangeWithAICostsUsesAgentRates(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	input := 1_000_000
	output := 500_000
	heartbeats := []Heartbeat{{
		Entity: "agent.go", Project: "api", Language: "Go", Time: float64(now.Add(-time.Hour).Unix()),
		AIInputTokens: &input, AIOutputTokens: &output, AISubscriptionPlan: "Codex",
	}}

	got, _, err := ComputeStatsForRangeWithAICosts(heartbeats, now, 15*time.Minute, "last_7_days", map[string]AICostRate{
		"Codex": {InputCostPerMillionCents: 100, OutputCostPerMillionCents: 400},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AI.EstimatedCostCents != 300 {
		t.Fatalf("expected 300 cents estimated cost, got %d", got.AI.EstimatedCostCents)
	}
	if got.AI.AIAgentCosts["Codex"] != 3 || got.AI.AIAgentTotalCost != 3 {
		t.Fatalf("expected WakaTime USD cost aliases, got costs=%#v total=%f", got.AI.AIAgentCosts, got.AI.AIAgentTotalCost)
	}
	if len(got.AI.Agents) != 1 || got.AI.Agents[0].EstimatedCostCents != 300 {
		t.Fatalf("expected agent-level cost, got %#v", got.AI.Agents)
	}
}

func TestComputeStatsForRangeWithAICostsAggregatesBeforeRounding(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	input := 500_000
	output := 0
	heartbeats := []Heartbeat{
		{
			Entity: "agent-a.go", Project: "api", Language: "Go", Time: float64(now.Add(-time.Hour).Unix()),
			AIInputTokens: &input, AIOutputTokens: &output, AISubscriptionPlan: "Codex",
		},
		{
			Entity: "agent-b.go", Project: "api", Language: "Go", Time: float64(now.Add(-30 * time.Minute).Unix()),
			AIInputTokens: &input, AIOutputTokens: &output, AISubscriptionPlan: "Codex",
		},
	}

	got, _, err := ComputeStatsForRangeWithAICosts(heartbeats, now, 15*time.Minute, "last_7_days", map[string]AICostRate{
		"Codex": {InputCostPerMillionCents: 1, OutputCostPerMillionCents: 0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AI.EstimatedCostCents != 1 {
		t.Fatalf("expected aggregate cost to preserve fractional cents until totals, got %d", got.AI.EstimatedCostCents)
	}
	if len(got.AI.Agents) != 1 || got.AI.Agents[0].EstimatedCostCents != 1 {
		t.Fatalf("expected agent-level aggregate cost, got %#v", got.AI.Agents)
	}
	if len(got.AI.Costs) != 1 || got.AI.Costs[0].WeeklyCents != 1 || got.AI.Costs[0].TotalCents != 1 {
		t.Fatalf("expected period aggregate cost, got %#v", got.AI.Costs)
	}
}

func TestComputeStatsForRangeWithAICostsPrefersModelForPricing(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	input := 1_000_000
	output := 500_000
	heartbeats := []Heartbeat{{
		Entity: "agent.go", Project: "api", Language: "Go", Time: float64(now.Add(-time.Hour).Unix()),
		AIInputTokens: &input, AIOutputTokens: &output, AIModel: "gpt-5.1-codex", AISubscriptionPlan: "pro",
	}}

	got, _, err := ComputeStatsForRangeWithAICosts(heartbeats, now, 15*time.Minute, "last_7_days", map[string]AICostRate{
		"pro":           {InputCostPerMillionCents: 1, OutputCostPerMillionCents: 1},
		"gpt-5.1-codex": {InputCostPerMillionCents: 100, OutputCostPerMillionCents: 400},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.AI.Agents) != 1 || got.AI.Agents[0].Name != "gpt-5.1-codex" {
		t.Fatalf("expected model-based AI row, got %#v", got.AI.Agents)
	}
	if got.AI.EstimatedCostCents != 300 {
		t.Fatalf("expected model-priced 300 cents estimated cost, got %d", got.AI.EstimatedCostCents)
	}
}

func TestComputeStatsForRangeWithAICostsFallsBackToProviderForPricing(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	input := 1_000_000
	output := 500_000
	heartbeats := []Heartbeat{{
		Entity: "agent.go", Project: "api", Language: "Go", Time: float64(now.Add(-time.Hour).Unix()),
		AIInputTokens: &input, AIOutputTokens: &output, AIProvider: "anthropic",
	}}

	got, _, err := ComputeStatsForRangeWithAICosts(heartbeats, now, 15*time.Minute, "last_7_days", map[string]AICostRate{
		"anthropic": {InputCostPerMillionCents: 300, OutputCostPerMillionCents: 1500},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.AI.Agents) != 1 || got.AI.Agents[0].Name != "anthropic" {
		t.Fatalf("expected provider-based AI row, got %#v", got.AI.Agents)
	}
	if got.AI.EstimatedCostCents != 1050 {
		t.Fatalf("expected provider-priced 1050 cents estimated cost, got %d", got.AI.EstimatedCostCents)
	}
}

func TestComputeStatsForRangeWithAICostsBuildsAgentCostPeriods(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	currentInput := 1_000_000
	currentOutput := 500_000
	oldInput := 1_000_000
	oldOutput := 0
	heartbeats := []Heartbeat{
		{
			Entity: "agent.go", Project: "api", Language: "Go", Time: float64(now.Add(-time.Hour).Unix()),
			AIInputTokens: &currentInput, AIOutputTokens: &currentOutput, AISubscriptionPlan: "Codex",
		},
		{
			Entity: "older.go", Project: "api", Language: "Go", Time: float64(now.AddDate(0, 0, -10).Unix()),
			AIInputTokens: &oldInput, AIOutputTokens: &oldOutput, AISubscriptionPlan: "Codex",
		},
	}

	got, _, err := ComputeStatsForRangeWithAICosts(heartbeats, now, 15*time.Minute, "last_30_days", map[string]AICostRate{
		"Codex": {InputCostPerMillionCents: 100, OutputCostPerMillionCents: 400},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.AI.Costs) != 1 {
		t.Fatalf("expected one agent cost row, got %#v", got.AI.Costs)
	}
	if got.AI.Costs[0].Agent != "Codex" || got.AI.Costs[0].DailyCents != 300 || got.AI.Costs[0].WeeklyCents != 300 || got.AI.Costs[0].MonthlyCents != 400 || got.AI.Costs[0].TotalCents != 400 {
		t.Fatalf("unexpected agent cost periods: %#v", got.AI.Costs[0])
	}
}

func TestComputeStatsForRangeCountsAIBucketSessionsOnce(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	aiLines := 10
	heartbeats := []Heartbeat{
		{
			Entity: "agent.go", Project: "api", Time: float64(now.Add(-2 * time.Hour).Unix()),
			AILineChanges: &aiLines, AISession: "session-a", AISubscriptionPlan: "Codex",
		},
		{
			Entity: "agent2.go", Project: "api", Time: float64(now.Add(-2*time.Hour + time.Minute).Unix()),
			AILineChanges: &aiLines, AISession: "session-a", AISubscriptionPlan: "Codex",
		},
		{
			Entity: "agent3.go", Project: "api", Time: float64(now.Add(-time.Hour).Unix()),
			AILineChanges: &aiLines, AISession: "session-b", AISubscriptionPlan: "Codex",
		},
	}

	got, _, err := ComputeStatsForRange(heartbeats, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.AI.SessionCount != 2 {
		t.Fatalf("expected global AI session count 2, got %d", got.AI.SessionCount)
	}
	if len(got.AI.Agents) != 1 || got.AI.Agents[0].SessionCount != 2 {
		t.Fatalf("expected agent session count 2, got %#v", got.AI.Agents)
	}
	latestDay := got.AI.Days[len(got.AI.Days)-1]
	if latestDay.SessionCount != 2 {
		t.Fatalf("expected day session count 2, got %#v", latestDay)
	}
}

func TestComputeStatsForRangeIncludesProjectAIMetrics(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	apiAILines := 90
	apiHumanLines := 30
	apiInput := 1_000_000
	apiOutput := 250_000
	apiPrompt := 700
	webAILines := 20
	webHumanLines := 40
	webInput := 500_000
	webOutput := 100_000
	webPrompt := 250
	heartbeats := []Heartbeat{
		{
			Entity: "api.go", Project: "api", Time: float64(now.Add(-2 * time.Hour).Unix()),
			AILineChanges: &apiAILines, HumanLineChanges: &apiHumanLines, AISession: "api-1",
			AIInputTokens: &apiInput, AIOutputTokens: &apiOutput, AIPromptLength: &apiPrompt, AISubscriptionPlan: "Codex",
		},
		{
			Entity: "api2.go", Project: "api", Time: float64(now.Add(-2*time.Hour + 10*time.Minute).Unix()),
			AISession: "api-1", AISubscriptionPlan: "Codex",
		},
		{
			Entity: "page.tsx", Project: "web", Time: float64(now.Add(-time.Hour).Unix()),
			AILineChanges: &webAILines, HumanLineChanges: &webHumanLines, AISession: "web-1",
			AIInputTokens: &webInput, AIOutputTokens: &webOutput, AIPromptLength: &webPrompt, AISubscriptionPlan: "Codex",
		},
		{
			Entity: "page2.tsx", Project: "web", Time: float64(now.Add(-time.Hour + 5*time.Minute).Unix()),
			AISession: "web-2", AISubscriptionPlan: "Codex",
		},
	}

	got, _, err := ComputeStatsForRangeWithAICosts(heartbeats, now, 15*time.Minute, "last_7_days", map[string]AICostRate{
		"Codex": {InputCostPerMillionCents: 100, OutputCostPerMillionCents: 400},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.ProjectAI) != 2 {
		t.Fatalf("expected two project AI rows, got %#v", got.ProjectAI)
	}
	if got.ProjectAI[0].Name != "api" || got.ProjectAI[0].AISeconds != 600 || got.ProjectAI[0].AILineChanges != 90 || got.ProjectAI[0].HumanLineChanges != 30 {
		t.Fatalf("expected api project AI row first with duration and line totals, got %#v", got.ProjectAI)
	}
	if got.ProjectAI[0].AIInputTokens != 1_000_000 || got.ProjectAI[0].AIOutputTokens != 250_000 || got.ProjectAI[0].AIPromptLength != 700 {
		t.Fatalf("expected api token and prompt totals, got %#v", got.ProjectAI[0])
	}
	if got.ProjectAI[0].SessionCount != 1 {
		t.Fatalf("expected duplicate api session counted once, got %#v", got.ProjectAI[0])
	}
	if got.ProjectAI[0].EstimatedCostCents != 200 {
		t.Fatalf("expected api estimated cost of 200 cents, got %#v", got.ProjectAI[0])
	}
	if got.ProjectAI[1].Name != "web" || got.ProjectAI[1].AISeconds != 300 || got.ProjectAI[1].SessionCount != 2 {
		t.Fatalf("expected web project row with two sessions, got %#v", got.ProjectAI[1])
	}
}

func TestComputeWeekdayStatsAggregatesDailyBucketsMondayFirst(t *testing.T) {
	days := []DailyStat{
		{Date: "2026-06-15", TotalSeconds: 600},
		{Date: "2026-06-16", TotalSeconds: 0},
		{Date: "2026-06-17", TotalSeconds: 300},
		{Date: "2026-06-18", TotalSeconds: 0},
		{Date: "2026-06-19", TotalSeconds: 120},
		{Date: "2026-06-20", TotalSeconds: 0},
		{Date: "2026-06-21", TotalSeconds: 900},
		{Date: "2026-06-22", TotalSeconds: 300},
	}

	got := ComputeWeekdayStats(days)

	if len(got) != 7 {
		t.Fatalf("expected seven weekday rows, got %#v", got)
	}
	if got[0].Name != "Monday" || got[0].TotalSeconds != 900 || got[0].ActiveDays != 2 || got[0].AverageSeconds != 450 || got[0].Text != "15 mins" {
		t.Fatalf("expected Monday aggregate first, got %#v", got[0])
	}
	if got[2].Name != "Wednesday" || got[2].TotalSeconds != 300 || got[2].ActiveDays != 1 {
		t.Fatalf("expected Wednesday aggregate, got %#v", got[2])
	}
	if got[6].Name != "Sunday" || got[6].TotalSeconds != 900 || got[6].ActiveDays != 1 || got[6].AverageText != "15 mins" {
		t.Fatalf("expected Sunday aggregate last, got %#v", got[6])
	}
}

func TestComputeDailyAverageTrendUsesCumulativeAverage(t *testing.T) {
	days := []DailyStat{
		{Date: "2026-06-17", TotalSeconds: 600},
		{Date: "2026-06-18", TotalSeconds: 0},
		{Date: "2026-06-19", TotalSeconds: 300},
	}

	got := ComputeDailyAverageTrend(days)

	if len(got) != 3 {
		t.Fatalf("expected three trend rows, got %#v", got)
	}
	if got[0].Date != "2026-06-17" || got[0].AverageSeconds != 600 || got[0].AverageText != "10 mins" || got[0].DayCount != 1 {
		t.Fatalf("expected first day average to equal first day total, got %#v", got[0])
	}
	if got[1].Date != "2026-06-18" || got[1].AverageSeconds != 300 || got[1].AverageText != "5 mins" || got[1].DayCount != 2 {
		t.Fatalf("expected second day average over two days, got %#v", got[1])
	}
	if got[2].Date != "2026-06-19" || got[2].AverageSeconds != 300 || got[2].TotalSeconds != 300 || got[2].Text != "5 mins" {
		t.Fatalf("expected third day cumulative average and daily total, got %#v", got[2])
	}
}

func TestComputeHourlyTimelineSplitsDurationsAcrossHourBoundaries(t *testing.T) {
	start := time.Date(2026, 6, 19, 9, 50, 0, 0, time.UTC)
	projectDurations := []Duration{
		{Name: "api", Project: "api", Time: float64(start.Unix()), DurationSeconds: 15 * 60},
		{Name: "web", Project: "web", Time: float64(start.Add(70 * time.Minute).Unix()), DurationSeconds: 5 * 60},
	}
	languageDurations := []Duration{
		{Name: "Go", Language: "Go", Time: float64(start.Unix()), DurationSeconds: 15 * 60},
		{Name: "TypeScript", Language: "TypeScript", Time: float64(start.Add(70 * time.Minute).Unix()), DurationSeconds: 5 * 60},
	}

	got := ComputeHourlyTimeline(projectDurations, languageDurations)

	if len(got) != 24 {
		t.Fatalf("expected 24 hourly buckets, got %d", len(got))
	}
	if got[9].TotalSeconds != 600 || got[9].Projects[0].Name != "api" || got[9].Projects[0].TotalSeconds != 600 {
		t.Fatalf("expected 10 minutes of api in 09:00 bucket, got %#v", got[9])
	}
	if got[10].TotalSeconds != 300 || got[10].Projects[0].Name != "api" || got[10].Languages[0].Name != "Go" {
		t.Fatalf("expected 5 minutes of api/go in 10:00 bucket, got %#v", got[10])
	}
	if got[11].TotalSeconds != 300 || got[11].Projects[0].Name != "web" || got[11].Languages[0].Name != "TypeScript" {
		t.Fatalf("expected 5 minutes of web/typescript in 11:00 bucket, got %#v", got[11])
	}
	if got[9].Text != "10 mins" || got[10].Text != "5 mins" {
		t.Fatalf("expected humanized hourly text, got %#v %#v", got[9], got[10])
	}
}
