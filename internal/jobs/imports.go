package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/services"
)

const TypeWakaTimeImport = "wakatime_import:process"

type HeartbeatImportPayload = services.Heartbeat

type WakaTimeImportPayload struct {
	UserID                 uuid.UUID                `json:"user_id"`
	Heartbeats             []HeartbeatImportPayload `json:"heartbeats"`
	DefaultEditor          string                   `json:"default_editor,omitempty"`
	DefaultEditorVersion   string                   `json:"default_editor_version,omitempty"`
	DefaultOperatingSystem string                   `json:"default_operating_system,omitempty"`
	DefaultArchitecture    string                   `json:"default_architecture,omitempty"`
	DefaultPlugin          string                   `json:"default_plugin,omitempty"`
	DefaultPluginVersion   string                   `json:"default_plugin_version,omitempty"`
}

func NewWakaTimeImportTask(userID uuid.UUID, heartbeats []HeartbeatImportPayload, defaults services.HeartbeatDefaults) (*asynq.Task, error) {
	payload, err := json.Marshal(WakaTimeImportPayload{
		UserID:                 userID,
		Heartbeats:             heartbeats,
		DefaultEditor:          defaults.Editor,
		DefaultEditorVersion:   defaults.EditorVersion,
		DefaultOperatingSystem: defaults.OperatingSystem,
		DefaultArchitecture:    defaults.Architecture,
		DefaultPlugin:          defaults.Plugin,
		DefaultPluginVersion:   defaults.PluginVersion,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeWakaTimeImport, payload), nil
}

func ParseWakaTimeImportTask(task *asynq.Task) (WakaTimeImportPayload, error) {
	var payload WakaTimeImportPayload
	err := json.Unmarshal(task.Payload(), &payload)
	return payload, err
}
