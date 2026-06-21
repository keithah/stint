package api

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

func TestWakaTimeStatusBarPayloadUsesSummaryShape(t *testing.T) {
	now := time.Date(2026, 6, 19, 16, 0, 0, 0, time.UTC)
	payload := wakaTimeStatusBarPayload(services.StatusBarStats{
		TotalSeconds:    3723,
		GrandTotalText:  "1 hr 2 mins",
		Project:         "stint",
		ProjectSeconds:  3000,
		ProjectText:     "50 mins",
		Language:        "Go",
		LanguageSeconds: 3000,
		LanguageText:    "50 mins",
	}, db.User{Timezone: "UTC"}, now)

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", payload["data"])
	}
	if _, ok := data["range"].(map[string]any); !ok {
		t.Fatalf("expected WakaTime range object, got %#v", data["range"])
	}
	grandTotal, ok := data["grand_total"].(map[string]any)
	if !ok {
		t.Fatalf("expected WakaTime grand_total object, got %#v", data["grand_total"])
	}
	if grandTotal["text"] != "1 hr 2 mins" {
		t.Fatalf("unexpected grand total text: %#v", grandTotal["text"])
	}
	if len(data["projects"].([]map[string]any)) != 1 {
		t.Fatalf("expected project counter in WakaTime payload, got %#v", data["projects"])
	}
}
