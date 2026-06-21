package jobs

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TypeLeaderboardUpdate = "leaderboard:update"

type LeaderboardUpdatePayload struct {
	Range string `json:"range"`
}

func NewLeaderboardUpdateTask(rangeName string) (*asynq.Task, error) {
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	payload, err := json.Marshal(LeaderboardUpdatePayload{Range: rangeName})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeLeaderboardUpdate, payload), nil
}

func ParseLeaderboardUpdateTask(task *asynq.Task) (LeaderboardUpdatePayload, error) {
	var payload LeaderboardUpdatePayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}
