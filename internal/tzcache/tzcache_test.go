package tzcache

import (
	"testing"
	"time"
)

func TestLocationCachesValidZonesAndFallsBackToUTC(t *testing.T) {
	first := Location("America/Los_Angeles")
	second := Location("America/Los_Angeles")
	if first != second {
		t.Fatal("expected repeated timezone lookups to return the cached location pointer")
	}
	if got := Location("bad/timezone"); got != time.UTC {
		t.Fatalf("expected invalid timezone to fall back to UTC, got %s", got)
	}
	if got := Location(""); got != time.UTC {
		t.Fatalf("expected empty timezone to fall back to UTC, got %s", got)
	}
}
