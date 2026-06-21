package services

import "testing"

func TestFilterWritesOnlyReturnsOriginalWhenDisabled(t *testing.T) {
	heartbeats := []Heartbeat{{Entity: "read.go"}, {Entity: "write.go", IsWrite: true}}

	got := FilterWritesOnly(heartbeats, false)

	if len(got) != 2 {
		t.Fatalf("expected all heartbeats when writes_only is disabled, got %d", len(got))
	}
}

func TestFilterWritesOnlyKeepsOnlyWriteHeartbeats(t *testing.T) {
	heartbeats := []Heartbeat{{Entity: "read.go"}, {Entity: "write.go", IsWrite: true}}

	got := FilterWritesOnly(heartbeats, true)

	if len(got) != 1 {
		t.Fatalf("expected one write heartbeat, got %d", len(got))
	}
	if got[0].Entity != "write.go" {
		t.Fatalf("expected write heartbeat, got %#v", got[0])
	}
}
