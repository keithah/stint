package dumps

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

type fakeStore struct {
	user       db.User
	dump       db.DataDump
	heartbeats []services.Heartbeat
	external   []db.ExternalDuration
	completed  string
}

func (s *fakeStore) UserByID(context.Context, uuid.UUID) (db.User, error) {
	return s.user, nil
}

func (s *fakeStore) GetDataDump(context.Context, uuid.UUID, uuid.UUID) (db.DataDump, error) {
	return s.dump, nil
}

func (s *fakeStore) AllHeartbeats(context.Context, uuid.UUID) ([]services.Heartbeat, error) {
	return s.heartbeats, nil
}

func (s *fakeStore) ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error) {
	return s.external, nil
}

func (s *fakeStore) CompleteDataDumpWithURL(_ context.Context, _ uuid.UUID, _ uuid.UUID, downloadURL string) (db.DataDump, error) {
	s.completed = downloadURL
	s.dump.Status = "Completed"
	s.dump.DownloadURL = downloadURL
	return s.dump, nil
}

func TestGenerateLocalHeartbeatsDumpWritesSnapshotAndCompletesMetadata(t *testing.T) {
	userID := uuid.New()
	dumpID := uuid.New()
	storagePath := t.TempDir()
	store := &fakeStore{
		user: db.User{ID: userID, TimeoutMinutes: 15},
		dump: db.DataDump{ID: dumpID, Type: "heartbeats"},
		heartbeats: []services.Heartbeat{{
			Entity:  "/tmp/main.go",
			Type:    "file",
			Time:    float64(time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC).Unix()),
			Project: "stint",
		}},
	}

	dump, err := GenerateLocal(context.Background(), store, config.Config{StoragePath: storagePath}, userID, dumpID, time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GenerateLocal returned error: %v", err)
	}

	if dump.DownloadURL != DownloadURL(dumpID) || store.completed != DownloadURL(dumpID) {
		t.Fatalf("expected completed download URL %q, got dump=%q store=%q", DownloadURL(dumpID), dump.DownloadURL, store.completed)
	}
	raw, err := os.ReadFile(LocalPath(config.Config{StoragePath: storagePath}, userID, dumpID))
	if err != nil {
		t.Fatalf("expected generated dump file: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected JSON dump payload, got %s", raw)
	}
	var decoded []services.Heartbeat
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected top-level heartbeat JSON array: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Project != "stint" {
		t.Fatalf("expected heartbeat dump snapshot, got %s", raw)
	}
}

func TestBuildPayloadDailyDumpReturnsTopLevelSummaryArray(t *testing.T) {
	userID := uuid.New()
	store := &fakeStore{
		user: db.User{ID: userID, TimeoutMinutes: 15},
		heartbeats: []services.Heartbeat{
			{
				Entity:  "/tmp/main.go",
				Type:    "file",
				Time:    float64(time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC).Unix()),
				Project: "stint",
			},
			{
				Entity:  "/tmp/other.go",
				Type:    "file",
				Time:    float64(time.Date(2026, 6, 19, 10, 10, 0, 0, time.UTC).Unix()),
				Project: "stint",
			},
		},
	}

	payload, err := BuildPayload(context.Background(), store, store.user, "daily", time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}

	var rawPayload any = payload
	rows, ok := rawPayload.([]map[string]any)
	if !ok {
		t.Fatalf("expected daily dump payload to be a top-level summary array, got %#v", payload)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one daily row, got %d", len(rows))
	}
	if _, ok := rows[0]["grand_total"]; !ok {
		t.Fatalf("expected daily summary row, got %#v", rows[0])
	}
}

func TestWriteLocalDumpPayloadContainsHeartbeatData(t *testing.T) {
	userID := uuid.New()
	dumpID := uuid.New()
	cfg := config.Config{StoragePath: t.TempDir()}
	payload := []services.Heartbeat{{Entity: "/tmp/main.go", Type: "file", Time: 1781887600}}

	path, err := WriteLocalPayload(cfg, userID, dumpID, payload)
	if err != nil {
		t.Fatalf("WriteLocalPayload returned error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected dump file to be written: %v", err)
	}
	var decoded []services.Heartbeat
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected top-level JSON array: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Entity != "/tmp/main.go" {
		t.Fatalf("expected heartbeat snapshot, got %s", raw)
	}
}

func TestWriteLocalPayloadUsesCompactJSON(t *testing.T) {
	userID := uuid.New()
	dumpID := uuid.New()
	cfg := config.Config{StoragePath: t.TempDir()}
	payload := []map[string]any{{
		"range": map[string]string{"date": "2026-05-21"},
	}}

	path, err := WriteLocalPayload(cfg, userID, dumpID, payload)
	if err != nil {
		t.Fatalf("WriteLocalPayload returned error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected dump file to be written: %v", err)
	}

	if !strings.Contains(string(raw), `"date":"2026-05-21"`) {
		t.Fatalf("expected compact JSON date fragment, got %s", raw)
	}
}
