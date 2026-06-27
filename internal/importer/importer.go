package importer

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

type HeartbeatStore interface {
	InsertHeartbeat(ctx context.Context, userID uuid.UUID, heartbeat services.Heartbeat) (services.Heartbeat, error)
	InsertHeartbeats(ctx context.Context, userID uuid.UUID, heartbeats []services.Heartbeat) ([]db.HeartbeatInsertResult, error)
}

type Result struct {
	Status     string `json:"status"`
	Inserted   int    `json:"inserted"`
	Duplicates int    `json:"duplicates"`
	Invalid    int    `json:"invalid"`
	Total      int    `json:"total"`
}

func QueuedResult(total int) Result {
	return Result{Status: "Queued", Total: total}
}

func ProcessHeartbeats(ctx context.Context, store HeartbeatStore, userID uuid.UUID, heartbeats []services.Heartbeat, defaults services.HeartbeatDefaults, now time.Time) (Result, error) {
	result := Result{Status: "Completed", Total: len(heartbeats)}
	valid := make([]services.Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		services.PrepareHeartbeat(&heartbeat, defaults)
		if err := services.ValidateHeartbeatAt(heartbeat, now); err != nil {
			result.Invalid++
			continue
		}
		valid = append(valid, heartbeat)
	}
	if len(valid) == 0 {
		return result, nil
	}
	rows, err := store.InsertHeartbeats(ctx, userID, valid)
	if err != nil {
		return Result{}, err
	}
	for _, row := range rows {
		switch {
		case row.Stored:
			result.Inserted++
		case row.Duplicate || errors.Is(row.Err, db.ErrDuplicateHeartbeat):
			result.Duplicates++
		case row.Err != nil:
			return Result{}, row.Err
		}
	}
	return result, nil
}
