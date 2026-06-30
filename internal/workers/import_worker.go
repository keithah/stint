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
	importRow, err := w.Store.GetWakaTimeImport(ctx, payload.UserID, payload.ImportID)
	if err != nil {
		return err
	}
	result, err := importer.ProcessHeartbeats(ctx, w.Store, payload.UserID, importRow.Heartbeats, services.HeartbeatDefaults{
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
	if err := w.Store.MarkWakaTimeImportProcessed(ctx, payload.UserID, payload.ImportID); err != nil {
		return err
	}
	if result.Inserted == 0 {
		return nil
	}
	user, err := w.Store.UserByID(ctx, payload.UserID)
	if err != nil {
		return err
	}
	stats := NewStatsWorker(w.Store, nil)
	return stats.RecomputeRanges(ctx, payload.UserID, jobs.DefaultStatsRanges(), user.TimeoutMinutes, user.WritesOnly)
}
