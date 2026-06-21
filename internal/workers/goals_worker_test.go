package workers

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
)

func TestComputeWorkerGoalProgressIncludesExternalDurations(t *testing.T) {
	windowStart := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	windowEnd := windowStart.AddDate(0, 0, 1)
	goal := services.Goal{
		Title:     "Planning",
		Delta:     "day",
		Seconds:   1800,
		Projects:  []string{"ops"},
		Languages: []string{"Markdown"},
		IsEnabled: true,
	}
	external := []services.ExternalDuration{{
		StartTime: float64(windowStart.Add(9 * time.Hour).Unix()),
		EndTime:   float64(windowStart.Add(9*time.Hour + 30*time.Minute).Unix()),
		Project:   "ops",
		Language:  "Markdown",
	}}

	progress := computeWorkerGoalProgress(goal, nil, external, windowStart, windowEnd, 15*time.Minute)

	if progress.ActualSeconds != 1800 {
		t.Fatalf("expected external duration to count toward worker goal evaluation, got %d", progress.ActualSeconds)
	}
	if !progress.IsComplete {
		t.Fatal("expected external-duration goal to be complete")
	}
}

func TestGoalsEvaluateScheduleRunsHourlyForUserLocalMidnightFiltering(t *testing.T) {
	if goalsEvaluateScheduleSpec != "@hourly" {
		t.Fatalf("expected scheduled goals evaluation to run hourly for timezone-local midnight checks, got %q", goalsEvaluateScheduleSpec)
	}
}

func TestShouldEvaluateGoalUserForScheduledTaskUsesUserLocalMidnight(t *testing.T) {
	payload := jobs.GoalsEvaluatePayload{Scheduled: true}
	now := time.Date(2026, 6, 19, 7, 5, 0, 0, time.UTC)

	if !shouldEvaluateGoalUserForTask(payload, db.User{Timezone: "America/Los_Angeles"}, now) {
		t.Fatal("expected America/Los_Angeles user to be due during local midnight hour")
	}
	if shouldEvaluateGoalUserForTask(payload, db.User{Timezone: "UTC"}, now) {
		t.Fatal("expected UTC user not to be due outside local midnight hour")
	}
}

func TestShouldEvaluateGoalUserForManualTaskIgnoresLocalMidnight(t *testing.T) {
	payload := jobs.GoalsEvaluatePayload{NowUnix: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC).Unix()}
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	if !shouldEvaluateGoalUserForTask(payload, db.User{Timezone: "UTC"}, now) {
		t.Fatal("expected manual goals evaluation to evaluate all users")
	}
}
