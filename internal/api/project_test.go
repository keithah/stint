package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
	"github.com/labstack/echo/v4"
)

func TestProjectDetailRangeDefaultsAndValidatesSupportedRanges(t *testing.T) {
	tests := map[string]string{
		"":              "last_30_days",
		"last_7_days":   "last_7_days",
		"last_30_days":  "last_30_days",
		"last_6_months": "last_6_months",
		"last_year":     "last_year",
		"all_time":      "all_time",
		"2026":          "2026",
		"2026-06":       "2026-06",
	}

	for input, want := range tests {
		got, err := projectDetailRange(input)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", input, err)
		}
		if got != want {
			t.Fatalf("%q: expected %q, got %q", input, want, got)
		}
	}

	if _, err := projectDetailRange("yesterday"); err == nil {
		t.Fatal("expected unsupported project detail range to return an error")
	}
}

func TestProjectCommitPageURLPreservesProjectBranchAndPage(t *testing.T) {
	got := projectCommitPageURL("stint api", "main branch", 2)
	want := "/api/v1/users/current/projects/stint%20api/commits?branch=main+branch&page=2"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	firstPage := projectCommitPageURL("stint api", "", 1)
	if firstPage != "/api/v1/users/current/projects/stint%20api/commits" {
		t.Fatalf("expected first page URL to omit page query, got %q", firstPage)
	}
}

func TestCommitProjectPayloadIncludesPublicMetadata(t *testing.T) {
	payload := commitProjectPayload(db.Project{
		ID:           uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		Name:         "stint",
		Color:        "#38bdf8",
		HasPublicURL: true,
		Badge:        "https://example.com/badges/stint.svg",
	})

	if payload["color"] != "#38bdf8" {
		t.Fatalf("expected color to be included, got %#v", payload["color"])
	}
	if payload["has_public_url"] != true {
		t.Fatalf("expected has_public_url to be included, got %#v", payload["has_public_url"])
	}
	if payload["badge"] != "https://example.com/badges/stint.svg" {
		t.Fatalf("expected badge to be included, got %#v", payload["badge"])
	}
}

func TestPositiveQueryIntFallsBackForInvalidValues(t *testing.T) {
	e := echo.New()
	tests := map[string]int{
		"/projects?page=3":         3,
		"/projects?page=0":         10,
		"/projects?page=-2":        10,
		"/projects?page=not-a-num": 10,
		"/projects":                10,
	}
	for target, want := range tests {
		req := httptest.NewRequest("GET", target, nil)
		got := positiveQueryInt(e.NewContext(req, httptest.NewRecorder()), "page", 10)
		if got != want {
			t.Fatalf("%s: expected %d, got %d", target, want, got)
		}
	}
}

func TestProjectStatsResolverReturnsFreshCacheWithoutRecomputing(t *testing.T) {
	userID := uuid.New()
	store := &projectStatsFakeStore{
		cached: services.Stats{TotalSeconds: 90, IsUpToDate: true},
		found:  true,
	}
	resolver := projectStatsResolver{Store: store}

	stats, err := resolver.ProjectStats(context.Background(), db.User{ID: userID, TimeoutMinutes: 15}, db.Project{Name: "stint"}, "last_30_days")
	if err != nil {
		t.Fatalf("ProjectStats returned error: %v", err)
	}
	if stats.TotalSeconds != 90 {
		t.Fatalf("expected cached stats, got %#v", stats)
	}
	if store.heartbeatsForRangeCalls != 0 || store.upsertCalls != 0 {
		t.Fatalf("fresh cache should not recompute or upsert, got heartbeats=%d upserts=%d", store.heartbeatsForRangeCalls, store.upsertCalls)
	}
}

func TestProjectStatsResolverRecomputesStaleCacheAndWritesFreshStats(t *testing.T) {
	userID := uuid.New()
	now := time.Now().Add(-5 * time.Minute).Unix()
	store := &projectStatsFakeStore{
		cached: services.Stats{TotalSeconds: 1, IsUpToDate: false},
		found:  true,
		externalRows: []db.ExternalDuration{{
			StartTime: float64(now),
			EndTime:   float64(now + 1800),
			Project:   "stint",
			Language:  "Go",
		}},
	}
	resolver := projectStatsResolver{Store: store}

	stats, err := resolver.ProjectStats(context.Background(), db.User{ID: userID, TimeoutMinutes: 15}, db.Project{Name: "stint"}, "last_30_days")
	if err != nil {
		t.Fatalf("ProjectStats returned error: %v", err)
	}
	if stats.TotalSeconds <= 0 {
		t.Fatalf("expected recomputed project stats, got %#v", stats)
	}
	if store.heartbeatsForRangeCalls != 1 {
		t.Fatalf("expected one range heartbeat load, got %d", store.heartbeatsForRangeCalls)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("expected recomputed stats to be cached, got %d upserts", store.upsertCalls)
	}
}

func TestProjectStatsResolverUsesUserTimezoneForRangeWindows(t *testing.T) {
	userID := uuid.New()
	store := &projectStatsFakeStore{}
	resolver := projectStatsResolver{Store: store}

	_, err := resolver.ProjectStats(context.Background(), db.User{ID: userID, Timezone: "America/Los_Angeles", TimeoutMinutes: 15}, db.Project{Name: "stint"}, "last_7_days")
	if err != nil {
		t.Fatalf("ProjectStats returned error: %v", err)
	}

	if store.rangeNow.Location().String() != "America/Los_Angeles" {
		t.Fatalf("expected heartbeats range to use user timezone, got %s", store.rangeNow.Location())
	}
	if store.externalStart.Location().String() != "America/Los_Angeles" || store.externalEnd.Location().String() != "America/Los_Angeles" {
		t.Fatalf("expected external duration window to use user timezone, got start=%s end=%s", store.externalStart.Location(), store.externalEnd.Location())
	}
}

type projectStatsFakeStore struct {
	cached                  services.Stats
	found                   bool
	heartbeats              []services.Heartbeat
	externalRows            []db.ExternalDuration
	heartbeatsForRangeCalls int
	upsertCalls             int
	rangeNow                time.Time
	externalStart           time.Time
	externalEnd             time.Time
}

func (s *projectStatsFakeStore) ProjectStatsCache(context.Context, uuid.UUID, string, string) (services.Stats, bool, error) {
	return s.cached, s.found, nil
}

func (s *projectStatsFakeStore) UpsertProjectStatsCache(context.Context, uuid.UUID, string, string, services.Stats) error {
	s.upsertCalls++
	return nil
}

func (s *projectStatsFakeStore) AllHeartbeats(context.Context, uuid.UUID) ([]services.Heartbeat, error) {
	return s.heartbeats, nil
}

func (s *projectStatsFakeStore) HeartbeatsForStatsRange(_ context.Context, _ uuid.UUID, now time.Time, _ string) ([]services.Heartbeat, error) {
	s.rangeNow = now
	s.heartbeatsForRangeCalls++
	return s.heartbeats, nil
}

func (s *projectStatsFakeStore) ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error) {
	return s.externalRows, nil
}

func (s *projectStatsFakeStore) ExternalDurationsBetween(_ context.Context, _ uuid.UUID, start time.Time, end time.Time) ([]db.ExternalDuration, error) {
	s.externalStart = start
	s.externalEnd = end
	return s.externalRows, nil
}

func (s *projectStatsFakeStore) AICostRates(context.Context, uuid.UUID) (map[string]services.AICostRate, error) {
	return nil, nil
}
