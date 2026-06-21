package jobs

import (
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const TypeGoalsEvaluate = "goals:evaluate"

type GoalsEvaluatePayload struct {
	NowUnix   int64 `json:"now_unix,omitempty"`
	Scheduled bool  `json:"scheduled,omitempty"`
}

func NewGoalsEvaluateTask(now time.Time) (*asynq.Task, error) {
	nowUnix := int64(0)
	if !now.IsZero() {
		nowUnix = now.Unix()
	}
	payload, err := json.Marshal(GoalsEvaluatePayload{NowUnix: nowUnix})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeGoalsEvaluate, payload), nil
}

func NewScheduledGoalsEvaluateTask() (*asynq.Task, error) {
	payload, err := json.Marshal(GoalsEvaluatePayload{Scheduled: true})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeGoalsEvaluate, payload), nil
}

func ParseGoalsEvaluateTask(task *asynq.Task) (GoalsEvaluatePayload, error) {
	var payload GoalsEvaluatePayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}

func GoalsEvaluateNow(payload GoalsEvaluatePayload) time.Time {
	if payload.NowUnix > 0 {
		return time.Unix(payload.NowUnix, 0).UTC()
	}
	return time.Now().UTC()
}
