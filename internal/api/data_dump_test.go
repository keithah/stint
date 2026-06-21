package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

func TestDailyDumpDateRangeIncludesOldestHeartbeat(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	old := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	start, end := dailyDumpDateRange([]services.Heartbeat{{Time: float64(old.Unix())}}, nil, now)

	if !sameDate(start, old) {
		t.Fatalf("expected start date %s, got %s", old.Format("2006-01-02"), start.Format("2006-01-02"))
	}
	if !sameDate(end, now) {
		t.Fatalf("expected end date %s, got %s", now.Format("2006-01-02"), end.Format("2006-01-02"))
	}
}

func TestSummaryRowsForRangeIncludesRequestedOldDate(t *testing.T) {
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	heartbeatAt := day.Add(10 * time.Hour)

	rows := summaryRowsForRange(
		[]services.Heartbeat{{
			Time:            float64(heartbeatAt.Unix()),
			Project:         "stint",
			Language:        "Go",
			Category:        "coding",
			Dependencies:    "pgx",
			Editor:          "vscode",
			MachineName:     "laptop",
			OperatingSystem: "linux",
		}},
		nil,
		day,
		day,
		15*time.Minute,
		allSummaryFields(),
	)

	if len(rows) != 1 {
		t.Fatalf("expected one daily row, got %d", len(rows))
	}
	date := rows[0]["range"].(map[string]string)["date"]
	if date != "2026-05-01" {
		t.Fatalf("expected row date 2026-05-01, got %s", date)
	}
	projects := rows[0]["projects"].([]services.SliceTotal)
	if len(projects) == 0 || projects[0].Name != "stint" {
		t.Fatalf("expected stint project summary, got %#v", projects)
	}
	categories := rows[0]["categories"].([]services.SliceTotal)
	if len(categories) == 0 || categories[0].Name != "coding" {
		t.Fatalf("expected coding category summary, got %#v", categories)
	}
	dependencies := rows[0]["dependencies"].([]services.SliceTotal)
	if len(dependencies) == 0 || dependencies[0].Name != "pgx" {
		t.Fatalf("expected pgx dependency summary, got %#v", dependencies)
	}
	editors := rows[0]["editors"].([]services.SliceTotal)
	if len(editors) == 0 || editors[0].Name != "vscode" {
		t.Fatalf("expected vscode editor summary, got %#v", editors)
	}
	machines := rows[0]["machines"].([]services.SliceTotal)
	if len(machines) == 0 || machines[0].Name != "laptop" {
		t.Fatalf("expected laptop machine summary, got %#v", machines)
	}
	operatingSystems := rows[0]["operating_systems"].([]services.SliceTotal)
	if len(operatingSystems) == 0 || operatingSystems[0].Name != "linux" {
		t.Fatalf("expected linux operating system summary, got %#v", operatingSystems)
	}
}

func TestNormalizeDataDumpType(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{name: "blank defaults to heartbeats", input: "", want: "heartbeats"},
		{name: "heartbeats", input: "heartbeats", want: "heartbeats"},
		{name: "daily", input: "daily", want: "daily"},
		{name: "trims whitespace", input: " daily ", want: "daily"},
		{name: "rejects unsupported type", input: "summaries", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDataDumpType(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDataDumpDownloadErrorRejectsProcessingAndExpiredDumps(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)
	future := now.Add(time.Hour)

	tests := []struct {
		name       string
		dump       db.DataDump
		wantStatus int
	}{
		{
			name:       "processing",
			dump:       db.DataDump{ID: uuid.New(), Status: "Pending", IsProcessing: true},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "expired",
			dump:       db.DataDump{ID: uuid.New(), Status: "Completed", ExpiresAt: &expired},
			wantStatus: http.StatusGone,
		},
		{
			name:       "completed future expiry",
			dump:       db.DataDump{ID: uuid.New(), Status: "Completed", ExpiresAt: &future},
			wantStatus: 0,
		},
		{
			name:       "completed no expiry",
			dump:       db.DataDump{ID: uuid.New(), Status: "Completed"},
			wantStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, blocked := dataDumpDownloadError(tt.dump, now)
			if tt.wantStatus == 0 {
				if blocked {
					t.Fatalf("expected dump to be downloadable, got status %d", status)
				}
				return
			}
			if !blocked || status != tt.wantStatus {
				t.Fatalf("expected blocked status %d, got status %d blocked=%v", tt.wantStatus, status, blocked)
			}
		})
	}
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
