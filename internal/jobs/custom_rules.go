package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeCustomRulesApply = "custom_rules:apply"

type CustomRulesApplyPayload struct {
	UserID uuid.UUID `json:"user_id"`
}

func NewCustomRulesApplyTask(userID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(CustomRulesApplyPayload{UserID: userID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCustomRulesApply, payload), nil
}

func ParseCustomRulesApplyTask(task *asynq.Task) (CustomRulesApplyPayload, error) {
	var payload CustomRulesApplyPayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}
