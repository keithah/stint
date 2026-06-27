package workers

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/goaleval"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
)

type GoalsWorker struct {
	Store goaleval.Store
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
	return goaleval.Evaluator{Store: w.Store}.Evaluate(ctx, now)
}

func (w GoalsWorker) EvaluateForTask(ctx context.Context, payload jobs.GoalsEvaluatePayload) (int, error) {
	return goaleval.Evaluator{Store: w.Store}.EvaluateForTask(ctx, payload)
}

func shouldEvaluateGoalUserForTask(payload jobs.GoalsEvaluatePayload, user db.User, now time.Time) bool {
	return goaleval.ShouldEvaluateUserForTask(payload, user, now)
}

func serviceGoal(goal db.Goal) services.Goal {
	return goaleval.ServiceGoal(goal)
}

func computeWorkerGoalProgress(goal services.Goal, heartbeats []services.Heartbeat, external []services.ExternalDuration, start, end time.Time, timeout time.Duration) services.GoalProgress {
	return goaleval.ComputeProgress(goal, heartbeats, external, start, end, timeout)
}
