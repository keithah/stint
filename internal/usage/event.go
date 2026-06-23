// Package usage defines the canonical AI usage event shared by the local
// collector (adapters) and the server ingest pipeline. It is deliberately
// independent of pricing and storage so adding an agent never touches those.
package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// BillingType distinguishes flat-rate subscription usage (zero marginal cost)
// from metered API usage.
type BillingType string

const (
	BillingUnknown      BillingType = ""
	BillingAPI          BillingType = "api"
	BillingSubscription BillingType = "subscription"
)

// Event is the normalized usage record every adapter emits. Token fields keep
// cache granularity because collapsing cache tokens is the main source of wrong
// cost. All token counts are absolute (not deltas).
type Event struct {
	// identity / dedup
	EventID   string `json:"event_id"`
	MessageID string `json:"message_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	// source
	Agent     string `json:"agent"`
	SessionID string `json:"session_id"`
	Project   string `json:"project,omitempty"`

	// model + tokens
	Model               string `json:"model"`
	InputTokens         int    `json:"input_tokens"`
	OutputTokens        int    `json:"output_tokens"`
	CacheCreate5mTokens int    `json:"cache_create_5m_tokens"`
	CacheCreate1hTokens int    `json:"cache_create_1h_tokens"`
	CacheReadTokens     int    `json:"cache_read_tokens"`
	ReasoningTokens     int    `json:"reasoning_tokens,omitempty"`

	// cost (provider-reported, optional)
	CostUSDProvided *float64 `json:"cost_usd_provided,omitempty"`

	// time: RFC3339 UTC; TZOffsetMinutes preserves the original local offset
	Timestamp       string `json:"timestamp"`
	TZOffsetMinutes int    `json:"tz_offset_minutes,omitempty"`

	// billing context
	BillingType BillingType `json:"billing_type,omitempty"`
}

// TotalTokens is the sum of every token type, for activity displays.
func (e Event) TotalTokens() int {
	return e.InputTokens + e.OutputTokens + e.CacheCreate5mTokens + e.CacheCreate1hTokens + e.CacheReadTokens + e.ReasoningTokens
}

// HasUsage reports whether the event carries any token counts. Adapters use
// this to skip non-usage lines (tool calls, system/user messages) rather than
// emitting zero-filled rows.
func (e Event) HasUsage() bool {
	return e.TotalTokens() > 0 || e.CostUSDProvided != nil
}

// ComputeEventID returns a stable dedup key. When both provider ids exist they
// uniquely identify the request; otherwise we hash the immutable shape of the
// record so the same logical request appearing in a resumed session, a summary
// line, or a duplicated transcript entry collapses to one event.
func ComputeEventID(e Event) string {
	if e.MessageID != "" && e.RequestID != "" {
		return "id:" + hashParts(e.MessageID, e.RequestID)
	}
	if e.MessageID != "" {
		return "msg:" + hashParts(e.Agent, e.MessageID)
	}
	if e.RequestID != "" {
		return "req:" + hashParts(e.Agent, e.RequestID)
	}
	return "h:" + hashParts(
		e.Agent,
		e.SessionID,
		e.Timestamp,
		e.Model,
		fmt.Sprint(e.InputTokens),
		fmt.Sprint(e.OutputTokens),
		fmt.Sprint(e.CacheCreate5mTokens),
		fmt.Sprint(e.CacheCreate1hTokens),
		fmt.Sprint(e.CacheReadTokens),
	)
}

// EnsureID fills EventID if the adapter did not set one.
func (e *Event) EnsureID() {
	if e.EventID == "" {
		e.EventID = ComputeEventID(*e)
	}
}

func hashParts(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:16])
}

// Dedup removes events sharing an EventID, keeping the first occurrence and
// preserving order. Adapters and the ingest path both call this; feeding the
// same data twice must not change totals.
func Dedup(events []Event) []Event {
	seen := make(map[string]struct{}, len(events))
	out := events[:0:0]
	for _, event := range events {
		if event.EventID == "" {
			event.EnsureID()
		}
		if _, ok := seen[event.EventID]; ok {
			continue
		}
		seen[event.EventID] = struct{}{}
		out = append(out, event)
	}
	return out
}
