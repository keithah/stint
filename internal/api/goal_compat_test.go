package api

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

func TestWakaTimeGoalPayloadIncludesChartDataForTodayGoalCLI(t *testing.T) {
	now := time.Date(2026, 6, 19, 16, 0, 0, 0, time.UTC)
	payload := wakaTimeGoalPayload(services.GoalProgress{
		Goal: services.Goal{
			ID:        "00000000-0000-4000-8000-000000000000",
			Title:     "Daily coding",
			Delta:     "day",
			Seconds:   3600,
			IsEnabled: true,
		},
		ActualSeconds:  3723,
		TargetSeconds:  3600,
		HumanReadable:  "1 hr 2 mins",
		TargetReadable: "1 hr",
	}, db.User{Timezone: "UTC"}, now)

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", payload["data"])
	}
	chartData, ok := data["chart_data"].([]map[string]any)
	if !ok {
		t.Fatalf("expected chart_data array, got %#v", data["chart_data"])
	}
	if len(chartData) != 1 {
		t.Fatalf("expected one chart_data entry, got %d", len(chartData))
	}
	if chartData[0]["actual_seconds_text"] != "1 hr 2 mins" {
		t.Fatalf("unexpected actual_seconds_text: %#v", chartData[0]["actual_seconds_text"])
	}
	if chartData[0]["goal_seconds_text"] != "1 hr" {
		t.Fatalf("unexpected goal_seconds_text: %#v", chartData[0]["goal_seconds_text"])
	}
	if _, ok := chartData[0]["range"].(map[string]any); !ok {
		t.Fatalf("expected range object, got %#v", chartData[0]["range"])
	}
}

func TestWakaTimeGoalStatusCoversTerminalStates(t *testing.T) {
	tests := []struct {
		name       string
		progress   services.GoalProgress
		wantStatus string
		wantReason string
	}{
		{name: "snoozed", progress: services.GoalProgress{IsSnoozed: true}, wantStatus: "snoozed", wantReason: "goal snoozed"},
		{name: "ignored", progress: services.GoalProgress{IsIgnored: true}, wantStatus: "ignored", wantReason: "ignored by goal settings"},
		{name: "complete", progress: services.GoalProgress{IsComplete: true}, wantStatus: "success", wantReason: "goal reached"},
		{name: "pending", progress: services.GoalProgress{}, wantStatus: "pending", wantReason: ""},
	}
	for _, tt := range tests {
		status, reason := wakaTimeGoalStatus(tt.progress)
		if status != tt.wantStatus || reason != tt.wantReason {
			t.Fatalf("%s: expected %q/%q, got %q/%q", tt.name, tt.wantStatus, tt.wantReason, status, reason)
		}
	}
}
