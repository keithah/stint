package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	TypeStatsRecompute        = "stats:recompute"
	TypeProjectStatsRecompute = "project_stats:recompute"
)

type StatsRecomputePayload struct {
	UserID uuid.UUID `json:"user_id"`
	Ranges []string  `json:"ranges"`
}

type ProjectStatsRecomputePayload struct {
	UserID  uuid.UUID `json:"user_id"`
	Project string    `json:"project"`
	Range   string    `json:"range"`
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

func NewProjectStatsRecomputeTask(userID uuid.UUID, project, rangeName string) (*asynq.Task, error) {
	payload, err := json.Marshal(ProjectStatsRecomputePayload{UserID: userID, Project: project, Range: rangeName})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeProjectStatsRecompute, payload), nil
}

func ParseProjectStatsRecomputeTask(task *asynq.Task) (ProjectStatsRecomputePayload, error) {
	var payload ProjectStatsRecomputePayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}
