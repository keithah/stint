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
	for _, heartbeat := range heartbeats {
		services.PrepareHeartbeat(&heartbeat, defaults)
		if err := services.ValidateHeartbeatAt(heartbeat, now); err != nil {
			result.Invalid++
			continue
		}
		if _, err := store.InsertHeartbeat(ctx, userID, heartbeat); err != nil {
			if errors.Is(err, db.ErrDuplicateHeartbeat) {
				result.Duplicates++
				continue
			}
			return Result{}, err
		}
		result.Inserted++
	}
	return result, nil
}
