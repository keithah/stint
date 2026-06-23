package usage

import "testing"

func TestComputeEventIDPrefersProviderIDs(t *testing.T) {
	a := Event{Agent: "claude-code", MessageID: "m1", RequestID: "r1", InputTokens: 10}
	b := Event{Agent: "claude-code", MessageID: "m1", RequestID: "r1", InputTokens: 999, Model: "different"}
	if ComputeEventID(a) != ComputeEventID(b) {
		t.Fatal("events with the same message+request id must share an event id regardless of other fields")
	}
}

func TestComputeEventIDHashesShapeWhenNoIDs(t *testing.T) {
	base := Event{Agent: "claude-code", SessionID: "s1", Timestamp: "2026-06-23T00:00:00Z", Model: "claude-sonnet-4-6", InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200}
	same := base
	if ComputeEventID(base) != ComputeEventID(same) {
		t.Fatal("identical shapes must hash equally")
	}
	diff := base
	diff.OutputTokens = 51
	if ComputeEventID(base) == ComputeEventID(diff) {
		t.Fatal("different token shapes must hash differently")
	}
}

func TestDedupIsIdempotent(t *testing.T) {
	events := []Event{
		{Agent: "claude-code", SessionID: "s1", Timestamp: "t1", Model: "m", InputTokens: 10, OutputTokens: 5},
		{Agent: "claude-code", SessionID: "s1", Timestamp: "t1", Model: "m", InputTokens: 10, OutputTokens: 5}, // dup
		{Agent: "claude-code", SessionID: "s1", Timestamp: "t2", Model: "m", InputTokens: 20, OutputTokens: 5},
	}
	once := Dedup(events)
	if len(once) != 2 {
		t.Fatalf("expected 2 unique events, got %d", len(once))
	}
	// Property: feeding the result (or the doubled input) again changes nothing.
	twice := Dedup(append(append([]Event{}, events...), events...))
	if len(twice) != 2 {
		t.Fatalf("expected dedup of doubled input to stay 2, got %d", len(twice))
	}
	sum := func(list []Event) int {
		total := 0
		for _, e := range list {
			total += e.TotalTokens()
		}
		return total
	}
	if sum(once) != sum(twice) {
		t.Fatalf("doubling input must not change totals: %d vs %d", sum(once), sum(twice))
	}
}

func TestHasUsageSkipsEmpty(t *testing.T) {
	if (Event{Agent: "x"}).HasUsage() {
		t.Fatal("event with no tokens and no cost must not count as usage")
	}
	if !(Event{InputTokens: 1}).HasUsage() {
		t.Fatal("event with tokens must count as usage")
	}
}
