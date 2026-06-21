package workers

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
)

type HeartbeatsWorker struct {
	Store *db.Store
}

func (w HeartbeatsWorker) HandleHeartbeatsPurgeTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseHeartbeatsPurgeTask(task)
	if err != nil {
		return err
	}
	if payload.RetentionDays <= 0 {
		now := time.Now().UTC()
		if payload.NowUnix > 0 {
			now = time.Unix(payload.NowUnix, 0).UTC()
		}
		_, err = w.Store.PurgeHeartbeatsByUserRetention(ctx, now)
		return err
	}
	cutoff, ok := jobs.HeartbeatsPurgeCutoff(payload)
	if !ok {
		return nil
	}
	_, err = w.Store.PurgeHeartbeatsBefore(ctx, cutoff)
	return err
}
