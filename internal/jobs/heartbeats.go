package jobs

import (
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const TypeHeartbeatsPurge = "heartbeats:purge"

type HeartbeatsPurgePayload struct {
	RetentionDays int   `json:"retention_days"`
	NowUnix       int64 `json:"now_unix,omitempty"`
}

func NewHeartbeatsPurgeTask(retentionDays int, now time.Time) (*asynq.Task, error) {
	nowUnix := int64(0)
	if !now.IsZero() {
		nowUnix = now.Unix()
	}
	payload, err := json.Marshal(HeartbeatsPurgePayload{RetentionDays: retentionDays, NowUnix: nowUnix})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeHeartbeatsPurge, payload), nil
}

func ParseHeartbeatsPurgeTask(task *asynq.Task) (HeartbeatsPurgePayload, error) {
	var payload HeartbeatsPurgePayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}

func HeartbeatsPurgeCutoff(payload HeartbeatsPurgePayload) (float64, bool) {
	if payload.RetentionDays <= 0 {
		return 0, false
	}
	now := time.Now().UTC()
	if payload.NowUnix > 0 {
		now = time.Unix(payload.NowUnix, 0).UTC()
	}
	return float64(now.AddDate(0, 0, -payload.RetentionDays).Unix()), true
}
