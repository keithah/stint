package services

import "testing"

func TestValidateExternalDurationRejectsInvalidInput(t *testing.T) {
	valid := ExternalDuration{
		ExternalID: "calendar-1",
		Provider:   "calendar",
		Entity:     "Planning",
		Type:       "meeting",
		StartTime:  1_700_000_000,
		EndTime:    1_700_000_600,
	}

	tests := []struct {
		name  string
		input ExternalDuration
	}{
		{name: "missing external id", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.ExternalID = "" })},
		{name: "missing provider", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.Provider = "" })},
		{name: "missing entity", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.Entity = "" })},
		{name: "missing type", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.Type = "" })},
		{name: "missing start time", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.StartTime = 0 })},
		{name: "end equals start", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.EndTime = duration.StartTime })},
		{name: "end before start", input: withExternalDuration(valid, func(duration *ExternalDuration) { duration.EndTime = duration.StartTime - 1 })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateExternalDuration(tt.input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateExternalDurationAcceptsValidInput(t *testing.T) {
	err := ValidateExternalDuration(ExternalDuration{
		ExternalID: "calendar-1",
		Provider:   "calendar",
		Entity:     "Planning",
		Type:       "meeting",
		StartTime:  1_700_000_000,
		EndTime:    1_700_000_600,
	})
	if err != nil {
		t.Fatalf("expected valid external duration, got %v", err)
	}
}

func TestExternalDurationsContributeToSliceTotals(t *testing.T) {
	external := []ExternalDuration{
		{Project: "planning", Language: "Markdown", StartTime: 1_700_000_000, EndTime: 1_700_000_600},
		{Project: "planning", Language: "Markdown", StartTime: 1_700_000_700, EndTime: 1_700_001_000},
		{Project: "review", Language: "Go", StartTime: 1_700_002_000, EndTime: 1_700_002_120},
	}

	got := ExternalDurationTotals(external, "project")

	if len(got) != 2 {
		t.Fatalf("expected 2 project totals, got %#v", got)
	}
	if got[0].Name != "planning" || got[0].TotalSeconds != 900 {
		t.Fatalf("expected planning total first with 900s, got %#v", got[0])
	}
	if got[1].Name != "review" || got[1].TotalSeconds != 120 {
		t.Fatalf("expected review total with 120s, got %#v", got[1])
	}
}

func TestRankLeaderboardEntriesSortsBySecondsThenUsername(t *testing.T) {
	rows := []LeaderboardEntry{
		{UserID: "u1", Username: "zara", TotalSeconds: 120},
		{UserID: "u2", Username: "alex", TotalSeconds: 300},
		{UserID: "u3", Username: "mira", TotalSeconds: 300},
	}

	got := RankLeaderboardEntries(rows)

	if got[0].Username != "alex" || got[0].Rank != 1 {
		t.Fatalf("expected alex rank 1, got %#v", got)
	}
	if got[1].Username != "mira" || got[1].Rank != 2 {
		t.Fatalf("expected mira rank 2, got %#v", got)
	}
	if got[2].Username != "zara" || got[2].Rank != 3 {
		t.Fatalf("expected zara rank 3, got %#v", got)
	}
}

func withExternalDuration(input ExternalDuration, mutate func(*ExternalDuration)) ExternalDuration {
	mutate(&input)
	return input
}
