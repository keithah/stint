package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeDataDumpProcess = "data_dump:process"

type DataDumpProcessPayload struct {
	UserID uuid.UUID `json:"user_id"`
	DumpID uuid.UUID `json:"dump_id"`
}

func NewDataDumpProcessTask(userID, dumpID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(DataDumpProcessPayload{UserID: userID, DumpID: dumpID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeDataDumpProcess, payload), nil
}

func ParseDataDumpProcessTask(task *asynq.Task) (DataDumpProcessPayload, error) {
	var payload DataDumpProcessPayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}
