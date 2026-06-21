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
	startDate, endDate := dailyDateRange(heartbeats, external, now)
	return summaryRowsForRange(heartbeats, external, startDate, endDate, time.Duration(user.TimeoutMinutes)*time.Minute), nil
}

func WriteLocalPayload(cfg config.Config, userID, dumpID uuid.UUID, payload any) (string, error) {
	path := LocalPath(cfg, userID, dumpID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func ReadLocalPayload(cfg config.Config, userID, dumpID uuid.UUID) ([]byte, error) {
	return os.ReadFile(LocalPath(cfg, userID, dumpID))
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

func summaryRowsForRange(heartbeats []services.Heartbeat, external []services.ExternalDuration, startDate, endDate time.Time, timeout time.Duration) []map[string]any {
	data := []map[string]any{}
	for day := startDate; !day.After(endDate); day = day.AddDate(0, 0, 1) {
		next := day.AddDate(0, 0, 1)
		var daily []services.Heartbeat
		for _, heartbeat := range heartbeats {
			t := time.Unix(int64(heartbeat.Time), 0).UTC()
			if !t.Before(day) && t.Before(next) {
				daily = append(daily, heartbeat)
			}
		}
		var dailyExternal []services.ExternalDuration
		for _, duration := range external {
			started := time.Unix(int64(duration.StartTime), 0).UTC()
			ended := time.Unix(int64(duration.EndTime), 0).UTC()
			if started.Before(next) && ended.After(day) {
				dailyExternal = append(dailyExternal, duration)
			}
		}
		stats, _, _ := services.ComputeStatsForRangeWithExternalDurations(daily, dailyExternal, day.Add(12*time.Hour), timeout, "last_7_days")
		data = append(data, map[string]any{
			"range": map[string]string{
				"date":  day.Format("2006-01-02"),
				"start": day.Format(time.RFC3339),
				"end":   next.Format(time.RFC3339),
			},
			"grand_total":       map[string]any{"total_seconds": stats.TotalSeconds, "text": services.HumanDuration(stats.TotalSeconds)},
			"projects":          stats.Projects,
			"languages":         stats.Languages,
			"categories":        stats.Categories,
			"dependencies":      stats.Dependencies,
			"editors":           stats.Editors,
			"machines":          stats.Machines,
			"operating_systems": stats.OperatingSystems,
		})
	}
	return data
}

func dailyDateRange(heartbeats []services.Heartbeat, external []services.ExternalDuration, now time.Time) (time.Time, time.Time) {
	start := utcDate(now)
	end := start
	expand := func(t time.Time) {
		day := utcDate(t)
		if day.Before(start) {
			start = day
		}
		if day.After(end) {
			end = day
		}
	}
	for _, heartbeat := range heartbeats {
		if heartbeat.Time > 0 {
			expand(time.Unix(int64(heartbeat.Time), 0).UTC())
		}
	}
	for _, duration := range external {
		if duration.StartTime > 0 {
			expand(time.Unix(int64(duration.StartTime), 0).UTC())
		}
		if duration.EndTime > 0 {
			expand(time.Unix(int64(duration.EndTime), 0).UTC())
		}
	}
	return start, end
}

func utcDate(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
