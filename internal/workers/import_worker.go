package workers

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/importer"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
)

type ImportWorker struct {
	Store *db.Store
}

func (w ImportWorker) HandleWakaTimeImportTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseWakaTimeImportTask(task)
	if err != nil {
		return err
	}
	result, err := importer.ProcessHeartbeats(ctx, w.Store, payload.UserID, payload.Heartbeats, services.HeartbeatDefaults{
		Plugin:          payload.DefaultPlugin,
		PluginVersion:   payload.DefaultPluginVersion,
		Editor:          payload.DefaultEditor,
		EditorVersion:   payload.DefaultEditorVersion,
		OperatingSystem: payload.DefaultOperatingSystem,
		Architecture:    payload.DefaultArchitecture,
	}, time.Now())
	if err != nil {
		return err
	}
	if result.Inserted == 0 {
		return nil
	}
	user, err := w.Store.UserByID(ctx, payload.UserID)
	if err != nil {
		return err
	}
	stats := StatsWorker{Store: w.Store}
	for _, rangeName := range jobs.DefaultStatsRanges() {
		if _, err := stats.RecomputeRange(ctx, payload.UserID, rangeName, user.TimeoutMinutes, user.WritesOnly); err != nil {
			return err
		}
	}
	return nil
}
