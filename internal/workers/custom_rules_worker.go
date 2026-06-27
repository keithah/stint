package workers

import (
	"context"
	"errors"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
)

type CustomRulesWorker struct {
	Store *db.Store
}

func (w CustomRulesWorker) HandleCustomRulesApplyTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseCustomRulesApplyTask(task)
	if err != nil {
		return err
	}
	total, err := w.Store.CountHeartbeats(ctx, payload.UserID)
	if err != nil {
		return err
	}
	progress, err := w.Store.GetCustomRulesProgress(ctx, payload.UserID)
	if err == nil && shouldSkipCustomRulesApply(progress.Status) {
		return nil
	}
	if err != nil {
		return err
	}
	progress, err = w.Store.SetCustomRulesProgressProcessing(ctx, payload.UserID, total)
	if err != nil {
		return err
	}
	if shouldSkipCustomRulesApply(progress.Status) {
		return nil
	}
	changed, deleted, err := w.Store.ApplyCustomRulesToHeartbeats(ctx, payload.UserID)
	if err != nil {
		if errors.Is(err, db.ErrCustomRulesAborted) {
			return nil
		}
		_, _ = w.Store.FailCustomRulesProgress(ctx, payload.UserID, err.Error())
		return err
	}
	if _, err := w.Store.CompleteCustomRulesProgress(ctx, payload.UserID, total, changed, deleted); err != nil {
		return err
	}
	if changed == 0 && deleted == 0 {
		return nil
	}
	user, err := w.Store.UserByID(ctx, payload.UserID)
	if err != nil {
		return err
	}
	stats := NewStatsWorker(w.Store, nil)
	return stats.RecomputeRanges(ctx, payload.UserID, jobs.DefaultStatsRanges(), user.TimeoutMinutes, user.WritesOnly)
}

func shouldSkipCustomRulesApply(status string) bool {
	return status == "Aborted"
}
