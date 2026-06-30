package goaleval

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/tzcache"
	"golang.org/x/sync/errgroup"
)

const userEvaluationConcurrency = 4

type Store interface {
	ListUsers(context.Context) ([]db.User, error)
	ListGoals(context.Context, uuid.UUID) ([]db.Goal, error)
	HeartbeatsBetween(context.Context, uuid.UUID, float64, float64) ([]services.Heartbeat, error)
	ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error)
	ExternalDurationsBetween(context.Context, uuid.UUID, time.Time, time.Time) ([]db.ExternalDuration, error)
	UpsertGoalEvaluation(context.Context, uuid.UUID, db.Goal, services.GoalProgress, time.Time, time.Time) (db.GoalEvaluation, error)
}

type Evaluator struct {
	Store Store
}

func (e Evaluator) Evaluate(ctx context.Context, now time.Time) (int, error) {
	return e.evaluate(ctx, now, func(db.User, time.Time) bool { return true })
}

func (e Evaluator) EvaluateForTask(ctx context.Context, payload jobs.GoalsEvaluatePayload) (int, error) {
	now := jobs.GoalsEvaluateNow(payload)
	return e.evaluate(ctx, now, func(user db.User, now time.Time) bool {
		return ShouldEvaluateUserForTask(payload, user, now)
	})
}

func (e Evaluator) evaluate(ctx context.Context, now time.Time, shouldEvaluateUser func(db.User, time.Time) bool) (int, error) {
	users, err := e.Store.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(userEvaluationConcurrency)
	var evaluated atomic.Int64
	for _, user := range users {
		if !shouldEvaluateUser(user, now) {
			continue
		}
		user := user
		group.Go(func() error {
			count, err := e.evaluateUser(groupCtx, user, now)
			if err != nil {
				return err
			}
			evaluated.Add(int64(count))
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return int(evaluated.Load()), err
	}
	return int(evaluated.Load()), nil
}

func (e Evaluator) evaluateUser(ctx context.Context, user db.User, now time.Time) (int, error) {
	evaluated := 0
	userNow := now
	if user.Timezone != "" {
		userNow = now.In(tzcache.Location(user.Timezone))
	}
	goals, err := e.Store.ListGoals(ctx, user.ID)
	if err != nil {
		return evaluated, err
	}
	dataStart, dataEnd, hasEnabled := EvaluationDataWindow(goals, userNow)
	var heartbeats []services.Heartbeat
	var external []services.ExternalDuration
	if hasEnabled {
		heartbeats, err = e.Store.HeartbeatsBetween(ctx, user.ID, float64(dataStart.Unix()), float64(dataEnd.Unix()))
		if err != nil {
			return evaluated, err
		}
		heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
		externalRows, err := e.Store.ExternalDurationsBetween(ctx, user.ID, dataStart, dataEnd)
		if err != nil {
			return evaluated, err
		}
		external = ExternalDurations(externalRows)
	}
	for _, goal := range goals {
		if !goal.IsEnabled {
			continue
		}
		start, end := services.GoalEvaluationWindow(goal.Delta, userNow)
		progress := ComputeProgress(ServiceGoal(goal), heartbeats, external, start, end, time.Duration(user.TimeoutMinutes)*time.Minute)
		if _, err := e.Store.UpsertGoalEvaluation(ctx, user.ID, goal, progress, start, end); err != nil {
			return evaluated, err
		}
		evaluated++
	}
	return evaluated, nil
}

func EvaluationDataWindow(goals []db.Goal, now time.Time) (time.Time, time.Time, bool) {
	var start time.Time
	var end time.Time
	hasWindow := false
	for _, goal := range goals {
		if !goal.IsEnabled {
			continue
		}
		windowStart, windowEnd := services.GoalEvaluationWindow(goal.Delta, now)
		if goal.ImproveByPercent != nil {
			windowDuration := windowEnd.Sub(windowStart)
			windowStart = windowStart.Add(-windowDuration)
		}
		if !hasWindow || windowStart.Before(start) {
			start = windowStart
		}
		if !hasWindow || windowEnd.After(end) {
			end = windowEnd
		}
		hasWindow = true
	}
	return start, end, hasWindow
}

func ShouldEvaluateUserForTask(payload jobs.GoalsEvaluatePayload, user db.User, now time.Time) bool {
	if !payload.Scheduled {
		return true
	}
	location := time.UTC
	if user.Timezone != "" {
		location = tzcache.Location(user.Timezone)
	}
	return now.In(location).Hour() == 0
}

func ServiceGoal(goal db.Goal) services.Goal {
	out := services.Goal{
		ID:               goal.ID.String(),
		Title:            goal.Title,
		CustomTitle:      goal.CustomTitle,
		Delta:            goal.Delta,
		Seconds:          goal.Seconds,
		Languages:        goal.Languages,
		Editors:          goal.Editors,
		Projects:         goal.Projects,
		IgnoreDays:       goal.IgnoreDays,
		IgnoreZeroDays:   goal.IgnoreZeroDays,
		ImproveByPercent: goal.ImproveByPercent,
		IsEnabled:        goal.IsEnabled,
		IsInverse:        goal.IsInverse,
		IsSnoozed:        goal.IsSnoozed,
		CreatedAt:        goal.CreatedAt.Format(time.RFC3339),
		ModifiedAt:       goal.ModifiedAt.Format(time.RFC3339),
	}
	if goal.SnoozeUntil != nil {
		out.SnoozeUntil = goal.SnoozeUntil.Format(time.RFC3339)
	}
	return out
}

func ComputeProgress(goal services.Goal, heartbeats []services.Heartbeat, external []services.ExternalDuration, start, end time.Time, timeout time.Duration) services.GoalProgress {
	return services.ComputeGoalProgressForWindowWithExternalDurations(goal, heartbeats, external, start, end, timeout)
}

func ExternalDurations(rows []db.ExternalDuration) []services.ExternalDuration {
	out := make([]services.ExternalDuration, 0, len(rows))
	for _, duration := range rows {
		out = append(out, services.ExternalDuration{
			ID:         duration.ID.String(),
			ExternalID: duration.ExternalID,
			Provider:   duration.Provider,
			Entity:     duration.Entity,
			Type:       duration.Type,
			Category:   duration.Category,
			StartTime:  duration.StartTime,
			EndTime:    duration.EndTime,
			Project:    duration.Project,
			Branch:     duration.Branch,
			Language:   duration.Language,
			Meta:       duration.Meta,
		})
	}
	return out
}
