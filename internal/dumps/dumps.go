package dumps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/summaryrows"
)

type Store interface {
	UserByID(ctx context.Context, userID uuid.UUID) (db.User, error)
	GetDataDump(ctx context.Context, userID, dumpID uuid.UUID) (db.DataDump, error)
	AllHeartbeats(ctx context.Context, userID uuid.UUID) ([]services.Heartbeat, error)
	ListExternalDurations(ctx context.Context, userID uuid.UUID) ([]db.ExternalDuration, error)
	CompleteDataDumpWithURL(ctx context.Context, userID, dumpID uuid.UUID, downloadURL string) (db.DataDump, error)
}

func GenerateLocal(ctx context.Context, store Store, cfg config.Config, userID, dumpID uuid.UUID, now time.Time) (db.DataDump, error) {
	user, err := store.UserByID(ctx, userID)
	if err != nil {
		return db.DataDump{}, err
	}
	dump, err := store.GetDataDump(ctx, userID, dumpID)
	if err != nil {
		return db.DataDump{}, err
	}
	payload, err := BuildPayload(ctx, store, user, dump.Type, now)
	if err != nil {
		return db.DataDump{}, err
	}
	if _, err := WriteLocalPayload(cfg, userID, dumpID, payload); err != nil {
		return db.DataDump{}, err
	}
	return store.CompleteDataDumpWithURL(ctx, userID, dumpID, DownloadURL(dumpID))
}

func BuildPayload(ctx context.Context, store Store, user db.User, dumpType string, now time.Time) (any, error) {
	heartbeats, err := store.AllHeartbeats(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if user.WritesOnly {
		heartbeats = services.FilterWritesOnly(heartbeats, true)
	}
	if dumpType != "daily" {
		return heartbeats, nil
	}
	externalRows, err := store.ListExternalDurations(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	external := toServiceExternalDurations(externalRows)
	startDate, endDate := summaryrows.DateRange(heartbeats, external, now)
	return summaryrows.RowsForRange(heartbeats, external, startDate, endDate, time.Duration(user.TimeoutMinutes)*time.Minute, summaryrows.AllFields()), nil
}

func WriteLocalPayload(cfg config.Config, userID, dumpID uuid.UUID, payload any) (string, error) {
	path := LocalPath(cfg, userID, dumpID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(payload); err != nil {
		return "", err
	}
	return path, nil
}

func LocalPath(cfg config.Config, userID, dumpID uuid.UUID) string {
	return filepath.Join(cfg.StoragePath, userID.String(), dumpID.String()+".json")
}

func DownloadURL(dumpID uuid.UUID) string {
	return fmt.Sprintf("/api/v1/users/current/data_dumps/%s/download", dumpID)
}

func toServiceExternalDurations(durations []db.ExternalDuration) []services.ExternalDuration {
	rows := make([]services.ExternalDuration, 0, len(durations))
	for _, duration := range durations {
		rows = append(rows, services.ExternalDuration{
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
	return rows
}
