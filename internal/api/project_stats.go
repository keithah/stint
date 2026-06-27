package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

type projectStatsStore interface {
	ProjectStatsCache(context.Context, uuid.UUID, string, string) (services.Stats, bool, error)
	UpsertProjectStatsCache(context.Context, uuid.UUID, string, string, services.Stats) error
	AllHeartbeats(context.Context, uuid.UUID) ([]services.Heartbeat, error)
	HeartbeatsForStatsRange(context.Context, uuid.UUID, time.Time, string) ([]services.Heartbeat, error)
	ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error)
	ExternalDurationsBetween(context.Context, uuid.UUID, time.Time, time.Time) ([]db.ExternalDuration, error)
	AICostRates(context.Context, uuid.UUID) (map[string]services.AICostRate, error)
}

type projectStatsResolver struct {
	Store       projectStatsStore
	BakeProject func(context.Context, uuid.UUID, *time.Location, string, string, *services.Stats)
}

func (r projectStatsResolver) ProjectStats(ctx context.Context, user db.User, project db.Project, rangeName string) (services.Stats, error) {
	if cached, found, err := r.Store.ProjectStatsCache(ctx, user.ID, project.Name, rangeName); err != nil {
		return services.Stats{}, err
	} else if found && cached.IsUpToDate {
		return cached, nil
	}

	location := userLocation(user)
	now := time.Now().In(location)
	var heartbeats []services.Heartbeat
	var externalRows []db.ExternalDuration
	var err error
	if rangeName == "all_time" {
		heartbeats, err = r.Store.AllHeartbeats(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
		externalRows, err = r.Store.ListExternalDurations(ctx, user.ID)
		if err != nil {
			return services.Stats{}, err
		}
	} else {
		heartbeats, err = r.Store.HeartbeatsForStatsRange(ctx, user.ID, now, rangeName)
		if err != nil {
			return services.Stats{}, err
		}
		window, err := services.WindowForRange(now, rangeName)
		if err != nil {
			return services.Stats{}, err
		}
		externalRows, err = r.Store.ExternalDurationsBetween(ctx, user.ID, window.Start, window.End)
		if err != nil {
			return services.Stats{}, err
		}
	}

	heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
	filtered := make([]services.Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		if heartbeat.Project == project.Name {
			filtered = append(filtered, heartbeat)
		}
	}
	projectExternal := []services.ExternalDuration{}
	for _, duration := range toServiceExternalDurations(externalRows) {
		if duration.Project == project.Name {
			projectExternal = append(projectExternal, duration)
		}
	}
	costs, err := r.Store.AICostRates(ctx, user.ID)
	if err != nil {
		return services.Stats{}, err
	}
	var stats services.Stats
	if rangeName == "all_time" {
		stats = services.ComputeAllTimeStatsWithExternalDurationsAndAICosts(filtered, projectExternal, time.Duration(user.TimeoutMinutes)*time.Minute, costs)
	} else {
		stats, _, err = services.ComputeStatsForRangeWithExternalDurationsAndAICosts(filtered, projectExternal, now, time.Duration(user.TimeoutMinutes)*time.Minute, rangeName, costs)
		if err != nil {
			return services.Stats{}, err
		}
	}
	if r.BakeProject != nil {
		r.BakeProject(ctx, user.ID, location, rangeName, project.Name, &stats)
	}
	if err := r.Store.UpsertProjectStatsCache(ctx, user.ID, project.Name, rangeName, stats); err != nil {
		return services.Stats{}, err
	}
	return stats, nil
}
