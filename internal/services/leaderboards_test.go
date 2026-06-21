package services

import "testing"

func TestNormalizeLeaderboardInputTrimsNameAndPreservesSupportedRange(t *testing.T) {
	name, rangeName := NormalizeLeaderboardInput("  Team  ", "last_30_days")
	if name != "Team" {
		t.Fatalf("expected trimmed name, got %q", name)
	}
	if rangeName != "last_30_days" {
		t.Fatalf("expected supported range to be preserved, got %q", rangeName)
	}
}

func TestNormalizeLeaderboardInputDefaultsBlankRange(t *testing.T) {
	_, rangeName := NormalizeLeaderboardInput("Team", "")
	if rangeName != "last_7_days" {
		t.Fatalf("expected blank range to default to last_7_days, got %q", rangeName)
	}
}

func TestValidateLeaderboardInputRejectsUnsupportedRange(t *testing.T) {
	if err := ValidateLeaderboardInput("Team", "yesterday"); err == nil {
		t.Fatal("expected unsupported range to be rejected")
	}
}

func TestValidateLeaderboardInputRejectsBlankName(t *testing.T) {
	if err := ValidateLeaderboardInput("   ", "last_7_days"); err == nil {
		t.Fatal("expected blank name to be rejected")
	}
}

func TestValidateLeaderboardInputAllowsBlankAndSupportedRanges(t *testing.T) {
	for _, rangeName := range []string{"", "last_7_days", "last_30_days", "last_6_months", "last_year", "all_time", "2026", "2026-06"} {
		if err := ValidateLeaderboardInput("Team", rangeName); err != nil {
			t.Fatalf("expected range %q to be valid, got %v", rangeName, err)
		}
	}
}
