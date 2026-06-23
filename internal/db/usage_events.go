package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/usage"
)

// UsageIngestResult summarizes a bulk usage ingest for the health panel.
type UsageIngestResult struct {
	Received   int `json:"received"`
	Inserted   int `json:"inserted"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
}

const usageInsertColumns = 18

// InsertUsageEvents stores canonical usage events, deduping within the batch by
// event id. Existing rows are upserted via ON CONFLICT DO UPDATE: re-ingesting an
// event updates the stored row to the latest token/cost values, so a corrected
// re-scan fixes totals rather than being dropped. Re-ingested rows are counted as
// duplicates (not inserts) via `RETURNING (xmax = 0)`, which is true only for
// freshly-inserted rows.
func (s *Store) InsertUsageEvents(ctx context.Context, userID uuid.UUID, events []usage.Event) (UsageIngestResult, error) {
	result := UsageIngestResult{Received: len(events)}
	seen := make(map[string]struct{}, len(events))
	type row struct {
		event usage.Event
		ts    time.Time
	}
	rows := make([]row, 0, len(events))
	for _, event := range events {
		event.EnsureID()
		if !event.HasUsage() {
			result.Invalid++
			continue
		}
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			result.Invalid++
			continue
		}
		if _, ok := seen[event.EventID]; ok {
			result.Duplicates++
			continue
		}
		seen[event.EventID] = struct{}{}
		rows = append(rows, row{event: event, ts: ts.UTC()})
	}

	const chunk = 100
	for start := 0; start < len(rows); start += chunk {
		end := start + chunk
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		placeholders := make([]string, 0, len(batch))
		args := make([]any, 0, len(batch)*usageInsertColumns)
		for i, r := range batch {
			base := i * usageInsertColumns
			ph := make([]string, usageInsertColumns)
			for j := 0; j < usageInsertColumns; j++ {
				ph[j] = fmt.Sprintf("$%d", base+j+1)
			}
			placeholders = append(placeholders, "("+strings.Join(ph, ",")+")")
			e := r.event
			args = append(args,
				userID, e.EventID, nullEmpty(e.MessageID), nullEmpty(e.RequestID), e.Agent,
				e.SessionID, nullEmpty(e.Project), e.Model,
				e.InputTokens, e.OutputTokens, e.CacheCreate5mTokens, e.CacheCreate1hTokens,
				e.CacheReadTokens, e.ReasoningTokens, e.CostUSDProvided, nullEmpty(string(e.BillingType)),
				r.ts, e.TZOffsetMinutes,
			)
		}
		// Upsert: a re-ingested event with corrected token/cost counts (e.g. the
		// Claude streaming-output reconciliation) must update the stored row, not
		// be dropped. `RETURNING (xmax = 0)` is true for freshly-inserted rows and
		// false for updated ones, so we still report inserted vs. duplicate.
		query := `INSERT INTO usage_events (
			user_id, event_id, message_id, request_id, agent, session_id, project, model,
			input_tokens, output_tokens, cache_create_5m_tokens, cache_create_1h_tokens,
			cache_read_tokens, reasoning_tokens, cost_usd_provided, billing_type, ts, tz_offset_minutes
		) VALUES ` + strings.Join(placeholders, ",") + `
		ON CONFLICT (user_id, event_id) DO UPDATE SET
			message_id = EXCLUDED.message_id, request_id = EXCLUDED.request_id,
			agent = EXCLUDED.agent, session_id = EXCLUDED.session_id, project = EXCLUDED.project,
			model = EXCLUDED.model, input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens, cache_create_5m_tokens = EXCLUDED.cache_create_5m_tokens,
			cache_create_1h_tokens = EXCLUDED.cache_create_1h_tokens, cache_read_tokens = EXCLUDED.cache_read_tokens,
			reasoning_tokens = EXCLUDED.reasoning_tokens, cost_usd_provided = EXCLUDED.cost_usd_provided,
			billing_type = EXCLUDED.billing_type, ts = EXCLUDED.ts, tz_offset_minutes = EXCLUDED.tz_offset_minutes
		RETURNING (xmax = 0)`
		rows, err := s.Pool.Query(ctx, query, args...)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var insertedRow bool
			if err := rows.Scan(&insertedRow); err != nil {
				rows.Close()
				return result, err
			}
			if insertedRow {
				result.Inserted++
			} else {
				result.Duplicates++
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return result, err
		}
	}
	return result, nil
}

// UsageEventsBetween returns stored events in [start, end) ordered by time.
func (s *Store) UsageEventsBetween(ctx context.Context, userID uuid.UUID, start, end time.Time) ([]usage.Event, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT event_id, coalesce(message_id, ''), coalesce(request_id, ''), agent, session_id,
			coalesce(project, ''), model, input_tokens, output_tokens, cache_create_5m_tokens,
			cache_create_1h_tokens, cache_read_tokens, reasoning_tokens, cost_usd_provided,
			coalesce(billing_type, ''), ts, tz_offset_minutes
		FROM usage_events
		WHERE user_id = $1 AND ts >= $2 AND ts < $3
		ORDER BY ts ASC`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []usage.Event
	for rows.Next() {
		var e usage.Event
		var ts time.Time
		var billing string
		var cost *float64
		if err := rows.Scan(&e.EventID, &e.MessageID, &e.RequestID, &e.Agent, &e.SessionID,
			&e.Project, &e.Model, &e.InputTokens, &e.OutputTokens, &e.CacheCreate5mTokens,
			&e.CacheCreate1hTokens, &e.CacheReadTokens, &e.ReasoningTokens, &cost,
			&billing, &ts, &e.TZOffsetMinutes); err != nil {
			return nil, err
		}
		e.CostUSDProvided = cost
		e.BillingType = usage.BillingType(billing)
		e.Timestamp = ts.UTC().Format(time.RFC3339)
		events = append(events, e)
	}
	return events, rows.Err()
}
