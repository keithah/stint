package workers

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
)

type GoalsWorker struct {
	Store *db.Store
}

func (w GoalsWorker) HandleGoalsEvaluateTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseGoalsEvaluateTask(task)
	if err != nil {
		return err
	}
	_, err = w.EvaluateForTask(ctx, payload)
	return err
}

func (w GoalsWorker) Evaluate(ctx context.Context, now time.Time) (int, error) {
	return w.evaluate(ctx, now, func(db.User, time.Time) bool { return true })
}

func (w GoalsWorker) EvaluateForTask(ctx context.Context, payload jobs.GoalsEvaluatePayload) (int, error) {
	now := jobs.GoalsEvaluateNow(payload)
	return w.evaluate(ctx, now, func(user db.User, now time.Time) bool {
		return shouldEvaluateGoalUserForTask(payload, user, now)
	})
}

func (w GoalsWorker) evaluate(ctx context.Context, now time.Time, shouldEvaluateUser func(db.User, time.Time) bool) (int, error) {
	users, err := w.Store.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	evaluated := 0
	for _, user := range users {
		if !shouldEvaluateUser(user, now) {
			continue
		}
		userNow := now
		if location, err := time.LoadLocation(user.Timezone); err == nil {
			userNow = now.In(location)
		}
		goals, err := w.Store.ListGoals(ctx, user.ID)
		if err != nil {
			return evaluated, err
		}
		for _, goal := range goals {
			if !goal.IsEnabled {
				continue
			}
			start, end := services.GoalEvaluationWindow(goal.Delta, userNow)
			heartbeats, err := w.Store.AllHeartbeats(ctx, user.ID)
			if err != nil {
				return evaluated, err
			}
			heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
			externalRows, err := w.Store.ListExternalDurations(ctx, user.ID)
			if err != nil {
				return evaluated, err
			}
			progress := computeWorkerGoalProgress(serviceGoal(goal), heartbeats, workerExternalDurations(externalRows), start, end, time.Duration(user.TimeoutMinutes)*time.Minute)
			if _, err := w.Store.UpsertGoalEvaluation(ctx, user.ID, goal, progress, start, end); err != nil {
				return evaluated, err
			}
			evaluated++
		}
	}
	return evaluated, nil
}

func shouldEvaluateGoalUserForTask(payload jobs.GoalsEvaluatePayload, user db.User, now time.Time) bool {
	if !payload.Scheduled {
		return true
	}
	location := time.UTC
	if user.Timezone != "" {
		if loaded, err := time.LoadLocation(user.Timezone); err == nil {
			location = loaded
		}
	}
	return now.In(location).Hour() == 0
}

func serviceGoal(goal db.Goal) services.Goal {
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

func computeWorkerGoalProgress(goal services.Goal, heartbeats []services.Heartbeat, external []services.ExternalDuration, start, end time.Time, timeout time.Duration) services.GoalProgress {
	return services.ComputeGoalProgressForWindowWithExternalDurations(goal, heartbeats, external, start, end, timeout)
}
