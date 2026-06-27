package workers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestGoalsWorkerLoadsHistoryOncePerUser(t *testing.T) {
	userID := uuid.New()
	store := &countingGoalsStore{
		users: []db.User{{
			ID:             userID,
			Timezone:       "UTC",
			TimeoutMinutes: 15,
		}},
		goals: []db.Goal{
			{ID: uuid.New(), Title: "Daily", Delta: "day", Seconds: 60, IsEnabled: true},
			{ID: uuid.New(), Title: "Weekly", Delta: "week", Seconds: 120, IsEnabled: true},
		},
	}
	worker := GoalsWorker{Store: store}

	evaluated, err := worker.Evaluate(context.Background(), time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if evaluated != 2 {
		t.Fatalf("expected 2 evaluated goals, got %d", evaluated)
	}
	if store.heartbeatsBetweenCalls != 1 {
		t.Fatalf("expected one bounded heartbeat load per user, got %d", store.heartbeatsBetweenCalls)
	}
	if store.externalDurationsBetweenCalls != 1 {
		t.Fatalf("expected one bounded external duration load per user, got %d", store.externalDurationsBetweenCalls)
	}
}

type countingGoalsStore struct {
	users                         []db.User
	goals                         []db.Goal
	heartbeatsBetweenCalls        int
	externalDurationsBetweenCalls int
}

func (s *countingGoalsStore) ListUsers(context.Context) ([]db.User, error) {
	return s.users, nil
}

func (s *countingGoalsStore) ListGoals(context.Context, uuid.UUID) ([]db.Goal, error) {
	return s.goals, nil
}

func (s *countingGoalsStore) AllHeartbeats(context.Context, uuid.UUID) ([]services.Heartbeat, error) {
	return nil, nil
}

func (s *countingGoalsStore) HeartbeatsBetween(context.Context, uuid.UUID, float64, float64) ([]services.Heartbeat, error) {
	s.heartbeatsBetweenCalls++
	return nil, nil
}

func (s *countingGoalsStore) ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error) {
	return nil, nil
}

func (s *countingGoalsStore) ExternalDurationsBetween(context.Context, uuid.UUID, time.Time, time.Time) ([]db.ExternalDuration, error) {
	s.externalDurationsBetweenCalls++
	return nil, nil
}

func (s *countingGoalsStore) UpsertGoalEvaluation(context.Context, uuid.UUID, db.Goal, services.GoalProgress, time.Time, time.Time) (db.GoalEvaluation, error) {
	return db.GoalEvaluation{}, nil
}
