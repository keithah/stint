package workers

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/dumps"
	"github.com/keithah/stint/internal/jobs"
)

type DumpWorker struct {
	Store  *db.Store
	Config config.Config
}

func (w DumpWorker) HandleDataDumpProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseDataDumpProcessTask(task)
	if err != nil {
		return err
	}
	if _, err := dumps.GenerateLocal(ctx, w.Store, w.Config, payload.UserID, payload.DumpID, time.Now().UTC()); err != nil {
		_ = w.Store.FailDataDump(ctx, payload.UserID, payload.DumpID)
		return err
	}
	return nil
}
