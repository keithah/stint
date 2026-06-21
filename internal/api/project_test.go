package api

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
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
