package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
)

func TestDayRangeInLocationUsesLocalMidnight(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	start, end, err := dayRangeInLocation("2026-06-18", location, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("dayRangeInLocation returned error: %v", err)
	}
	wantStart := time.Date(2026, 6, 18, 0, 0, 0, 0, location)
	wantEnd := wantStart.AddDate(0, 0, 1)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("expected %s-%s, got %s-%s", wantStart, wantEnd, start, end)
	}
}

func TestDayRangeInLocationDefaultsToLocalToday(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 6, 19, 2, 0, 0, 0, time.UTC)
	start, _, err := dayRangeInLocation("", location, now)
	if err != nil {
		t.Fatalf("dayRangeInLocation returned error: %v", err)
	}
	if got := start.Format("2006-01-02"); got != "2026-06-18" {
		t.Fatalf("expected local date 2026-06-18, got %s", got)
	}
}

func TestDayRangeDefaultsAndValidatesDate(t *testing.T) {
	start, end, err := dayRange("2026-06-18")
	if err != nil {
		t.Fatalf("dayRange returned error: %v", err)
	}
	if end-start != 24*60*60 {
		t.Fatalf("expected one-day range, got start=%f end=%f", start, end)
	}

	if _, _, err := dayRange("06/18/2026"); err == nil {
		t.Fatal("expected invalid day to return an error")
	}
}

func TestDateRangeInLocationUsesLocalMidnights(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	start, end, err := dateRangeInLocation("2026-06-18", "2026-06-19", location, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("dateRangeInLocation returned error: %v", err)
	}
	if !start.Equal(time.Date(2026, 6, 18, 0, 0, 0, 0, location)) {
		t.Fatalf("unexpected start: %s", start)
	}
	if !end.Equal(time.Date(2026, 6, 19, 0, 0, 0, 0, location)) {
		t.Fatalf("unexpected end: %s", end)
	}
}

func TestDateRangeDefaultsAndValidation(t *testing.T) {
	start, end, err := dateRange("2026-06-18", "2026-06-19")
	if err != nil {
		t.Fatalf("dateRange returned error: %v", err)
	}
	if !start.Equal(time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)) || !end.Equal(time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected UTC range: %s-%s", start, end)
	}

	tests := map[string][2]string{
		"bad start": {"not-a-date", "2026-06-19"},
		"bad end":   {"2026-06-18", "not-a-date"},
		"reversed":  {"2026-06-19", "2026-06-18"},
		"too large": {"2025-01-01", "2026-06-19"},
	}
	for name, values := range tests {
		if _, _, err := dateRange(values[0], values[1]); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestUserLocationFallsBackToUTC(t *testing.T) {
	if got := userLocation(db.User{Timezone: "bad/timezone"}); got.String() != "UTC" {
		t.Fatalf("expected UTC fallback, got %s", got)
	}
}

func TestStatusCacheKeyIncludesInputsThatAffectTodayStats(t *testing.T) {
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000123")
	user := db.User{ID: userID, Timezone: "America/Los_Angeles", TimeoutMinutes: 30, WritesOnly: true}
	now := time.Date(2026, 6, 19, 2, 0, 0, 0, time.UTC)

	got := statusCacheKey(user, now)

	if got != "00000000-0000-4000-8000-000000000123:timezone:America/Los_Angeles:timeout:30:writes_only:true:date:2026-06-18" {
		t.Fatalf("expected cache key to include local date and status inputs, got %q", got)
	}
}
