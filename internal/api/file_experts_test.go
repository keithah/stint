package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

func TestWakaTimeFileExpertsPayloadIncludesCurrentUserRow(t *testing.T) {
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000000")
	payload := wakaTimeFileExpertsPayload(db.User{
		ID:             userID,
		GitHubUsername: "local-dev",
		FullName:       "Local Dev",
	}, 2409)

	data, ok := payload["data"].([]map[string]any)
	if !ok {
		t.Fatalf("expected data array, got %#v", payload["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected one current-user expert row, got %d", len(data))
	}
	total, ok := data[0]["total"].(map[string]any)
	if !ok {
		t.Fatalf("expected total object, got %#v", data[0]["total"])
	}
	if total["text"] != "40 mins" || total["total_seconds"] != float64(2409) {
		t.Fatalf("unexpected total: %#v", total)
	}
	expert, ok := data[0]["user"].(map[string]any)
	if !ok {
		t.Fatalf("expected user object, got %#v", data[0]["user"])
	}
	if expert["is_current_user"] != true || expert["id"] != userID.String() {
		t.Fatalf("unexpected expert user: %#v", expert)
	}
}

func TestFileExpertsTotalSecondsFiltersEntityAndProject(t *testing.T) {
	project := "stint"
	heartbeats := []services.Heartbeat{
		{Entity: "/tmp/stint/main.go", Project: "stint", Time: 1000},
		{Entity: "/tmp/stint/main.go", Project: "stint", Time: 1120},
		{Entity: "/tmp/stint/main.go", Project: "other", Time: 1180},
		{Entity: "/tmp/stint/other.go", Project: "stint", Time: 1240},
	}

	total := fileExpertsTotalSeconds(heartbeats, "/tmp/stint/main.go", &project, 15*time.Minute)

	if total != 120 {
		t.Fatalf("expected only matching entity/project duration, got %d", total)
	}
}
