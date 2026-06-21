package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeStatsRecompute = "stats:recompute"

type StatsRecomputePayload struct {
	UserID uuid.UUID `json:"user_id"`
	Ranges []string  `json:"ranges"`
}

func DefaultStatsRanges() []string {
	return []string{"last_7_days", "last_30_days", "last_6_months", "last_year", "all_time"}
}

func NewStatsRecomputeTask(userID uuid.UUID, ranges []string) (*asynq.Task, error) {
	if len(ranges) == 0 {
		ranges = DefaultStatsRanges()
	}
	payload, err := json.Marshal(StatsRecomputePayload{UserID: userID, Ranges: ranges})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeStatsRecompute, payload), nil
}

func ParseStatsRecomputeTask(task *asynq.Task) (StatsRecomputePayload, error) {
	var payload StatsRecomputePayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}
