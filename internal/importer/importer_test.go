package importer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
)

type fakeHeartbeatStore struct {
	seen map[string]bool
}

func (s *fakeHeartbeatStore) InsertHeartbeat(_ context.Context, _ uuid.UUID, heartbeat services.Heartbeat) (services.Heartbeat, error) {
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	key := heartbeat.Entity
	if s.seen[key] {
		return services.Heartbeat{}, db.ErrDuplicateHeartbeat
	}
	s.seen[key] = true
	return heartbeat, nil
}

func TestProcessHeartbeatsCountsInsertedDuplicatesAndInvalid(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeHeartbeatStore{}
	userID := uuid.New()

	result, err := ProcessHeartbeats(context.Background(), store, userID, []services.Heartbeat{
		{Entity: "/tmp/one.go", Time: float64(now.Unix())},
		{Entity: "/tmp/one.go", Time: float64(now.Unix())},
		{Entity: "", Time: float64(now.Unix())},
	}, services.HeartbeatDefaults{Editor: "vscode", OperatingSystem: "linux"}, now)
	if err != nil {
		t.Fatalf("ProcessHeartbeats returned error: %v", err)
	}

	if result.Status != "Completed" {
		t.Fatalf("expected completed result, got %q", result.Status)
	}
	if result.Inserted != 1 || result.Duplicates != 1 || result.Invalid != 1 || result.Total != 3 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestProcessHeartbeatsReturnsStoreErrors(t *testing.T) {
	store := failingHeartbeatStore{err: errors.New("database unavailable")}
	_, err := ProcessHeartbeats(context.Background(), store, uuid.New(), []services.Heartbeat{
		{Entity: "/tmp/one.go", Time: float64(time.Now().Unix())},
	}, services.HeartbeatDefaults{}, time.Now())
	if err == nil {
		t.Fatal("expected store error")
	}
}

type failingHeartbeatStore struct {
	err error
}

func (s failingHeartbeatStore) InsertHeartbeat(context.Context, uuid.UUID, services.Heartbeat) (services.Heartbeat, error) {
	return services.Heartbeat{}, s.err
}
