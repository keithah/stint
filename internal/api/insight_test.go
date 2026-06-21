package api

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/keithah/stint/internal/services"
)

func TestInsightDataReturnsFullStatsForStatsType(t *testing.T) {
	stats := services.Stats{
		Range:               "last_30_days",
		TotalSeconds:        1234,
		HumanReadableTotal:  "20 mins",
		DailyAverageSeconds: 41,
		HumanReadableDaily:  "41 secs",
		Projects: []services.SliceTotal{
			{Name: "stint", TotalSeconds: 1234, Text: "20 mins"},
		},
	}

	data, ok := insightData("stats", stats)

	if !ok {
		t.Fatal("expected stats insight type to be supported")
	}
	if !reflect.DeepEqual(data, stats) {
		t.Fatalf("expected stats insight to return full stats aggregate, got %#v", data)
	}
}

func TestInsightDataReturnsDependencies(t *testing.T) {
	stats := services.Stats{
		Dependencies: []services.SliceTotal{
			{Name: "pgx", TotalSeconds: 360, Text: "6 mins"},
		},
	}

	data, ok := insightData("dependencies", stats)

	if !ok {
		t.Fatal("expected dependencies insight type to be supported")
	}
	if !reflect.DeepEqual(data, stats.Dependencies) {
		t.Fatalf("expected dependencies insight to return dependencies aggregate, got %#v", data)
	}
}

func TestInsightDataSupportsEveryDocumentedType(t *testing.T) {
	stats := services.Stats{
		Projects:         []services.SliceTotal{{Name: "stint"}},
		Languages:        []services.SliceTotal{{Name: "Go"}},
		Editors:          []services.SliceTotal{{Name: "vscode"}},
		Machines:         []services.SliceTotal{{Name: "workstation"}},
		OperatingSystems: []services.SliceTotal{{Name: "linux"}},
		Categories:       []services.SliceTotal{{Name: "coding"}},
		Dependencies:     []services.SliceTotal{{Name: "pgx"}},
		Days:             []services.DailyStat{{Date: "2026-06-19", TotalSeconds: 120}},
		Hourly:           []services.HourlyStat{{Hour: 10, TotalSeconds: 120}},
		BestDay:          services.DailyStat{Date: "2026-06-19", TotalSeconds: 120},
		AI: services.AIMetrics{
			Agents: []services.AIStat{{Name: "Codex", AISeconds: 30}},
			Days:   []services.AIStat{{Name: "2026-06-19", AISeconds: 30}},
		},
		DailyAverageSeconds: 17,
		HumanReadableDaily:  "17 secs",
	}

	for _, insightType := range supportedInsightTypes() {
		if _, ok := insightData(insightType, stats); !ok {
			t.Fatalf("expected insight type %q to be supported", insightType)
		}
	}
	if _, ok := insightData("unknown", stats); ok {
		t.Fatal("expected unknown insight type to be rejected")
	}
}

func TestSpecDocumentsSupportedInsightTypes(t *testing.T) {
	raw, err := os.ReadFile("../../docs/SPEC.md")
	if err != nil {
		t.Fatalf("could not read spec: %v", err)
	}
	spec := string(raw)
	insightsRow := ""
	for _, line := range strings.Split(spec, "\n") {
		if strings.Contains(line, "/api/v1/users/current/insights/:insight_type/:range") {
			insightsRow = line
			break
		}
	}
	if insightsRow == "" {
		t.Fatal("docs/SPEC.md should document the insights endpoint")
	}
	for _, insightType := range []string{
		"stats",
		"projects",
		"languages",
		"editors",
		"machines",
		"operating_systems",
		"categories",
		"dependencies",
		"days",
		"hours",
		"weekdays",
		"best_day",
		"daily_average",
		"daily_average_trend",
		"ai_agents",
		"ai_days",
	} {
		if !strings.Contains(insightsRow, insightType) {
			t.Fatalf("docs/SPEC.md should document insight type %q", insightType)
		}
	}
}
