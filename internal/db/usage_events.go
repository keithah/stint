package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/usage"
	"github.com/keithah/stint/internal/usagestats"
)

// UsageIngestResult summarizes a bulk usage ingest for the health panel.
type UsageIngestResult struct {
	Received   int `json:"received"`
	Inserted   int `json:"inserted"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
}

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

	eventIDs := make([]string, 0, len(rows))
	messageIDs := make([]string, 0, len(rows))
	requestIDs := make([]string, 0, len(rows))
	agents := make([]string, 0, len(rows))
	sessionIDs := make([]string, 0, len(rows))
	projects := make([]string, 0, len(rows))
	models := make([]string, 0, len(rows))
	inputTokens := make([]int, 0, len(rows))
	outputTokens := make([]int, 0, len(rows))
	cacheCreate5mTokens := make([]int, 0, len(rows))
	cacheCreate1hTokens := make([]int, 0, len(rows))
	cacheReadTokens := make([]int, 0, len(rows))
	reasoningTokens := make([]int, 0, len(rows))
	costs := make([]*float64, 0, len(rows))
	billingTypes := make([]string, 0, len(rows))
	timestamps := make([]time.Time, 0, len(rows))
	tzOffsets := make([]int, 0, len(rows))
	for _, r := range rows {
		e := r.event
		eventIDs = append(eventIDs, e.EventID)
		messageIDs = append(messageIDs, e.MessageID)
		requestIDs = append(requestIDs, e.RequestID)
		agents = append(agents, e.Agent)
		sessionIDs = append(sessionIDs, e.SessionID)
		projects = append(projects, e.Project)
		models = append(models, e.Model)
		inputTokens = append(inputTokens, e.InputTokens)
		outputTokens = append(outputTokens, e.OutputTokens)
		cacheCreate5mTokens = append(cacheCreate5mTokens, e.CacheCreate5mTokens)
		cacheCreate1hTokens = append(cacheCreate1hTokens, e.CacheCreate1hTokens)
		cacheReadTokens = append(cacheReadTokens, e.CacheReadTokens)
		reasoningTokens = append(reasoningTokens, e.ReasoningTokens)
		costs = append(costs, e.CostUSDProvided)
		billingTypes = append(billingTypes, string(e.BillingType))
		timestamps = append(timestamps, r.ts)
		tzOffsets = append(tzOffsets, e.TZOffsetMinutes)
	}

	const query = `WITH input AS (
		SELECT $1::uuid AS user_id, *
		FROM unnest(
			$2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[],
			$9::int[], $10::int[], $11::int[], $12::int[], $13::int[], $14::int[],
			$15::double precision[], $16::text[], $17::timestamptz[], $18::int[]
		) AS t(
			event_id, message_id, request_id, agent, session_id, project, model,
			input_tokens, output_tokens, cache_create_5m_tokens, cache_create_1h_tokens,
			cache_read_tokens, reasoning_tokens, cost_usd_provided, billing_type, ts, tz_offset_minutes
		)
	)
	INSERT INTO usage_events (
			user_id, event_id, message_id, request_id, agent, session_id, project, model,
			input_tokens, output_tokens, cache_create_5m_tokens, cache_create_1h_tokens,
			cache_read_tokens, reasoning_tokens, cost_usd_provided, billing_type, ts, tz_offset_minutes
		)
		SELECT user_id, event_id, nullif(message_id, ''), nullif(request_id, ''), agent,
			session_id, nullif(project, ''), model, input_tokens, output_tokens,
			cache_create_5m_tokens, cache_create_1h_tokens, cache_read_tokens,
			reasoning_tokens, cost_usd_provided, nullif(billing_type, ''), ts, tz_offset_minutes
		FROM input
		ON CONFLICT (user_id, event_id) DO UPDATE SET
			message_id = EXCLUDED.message_id, request_id = EXCLUDED.request_id,
			agent = EXCLUDED.agent, session_id = EXCLUDED.session_id, project = EXCLUDED.project,
		model = EXCLUDED.model, input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens, cache_create_5m_tokens = EXCLUDED.cache_create_5m_tokens,
			cache_create_1h_tokens = EXCLUDED.cache_create_1h_tokens, cache_read_tokens = EXCLUDED.cache_read_tokens,
			reasoning_tokens = EXCLUDED.reasoning_tokens, cost_usd_provided = EXCLUDED.cost_usd_provided,
			billing_type = EXCLUDED.billing_type, ts = EXCLUDED.ts, tz_offset_minutes = EXCLUDED.tz_offset_minutes
		RETURNING (xmax = 0)`

	resultRows, err := s.Pool.Query(ctx, query,
		userID, eventIDs, messageIDs, requestIDs, agents, sessionIDs, projects, models,
		inputTokens, outputTokens, cacheCreate5mTokens, cacheCreate1hTokens,
		cacheReadTokens, reasoningTokens, costs, billingTypes, timestamps, tzOffsets,
	)
	if err != nil {
		return result, err
	}
	defer resultRows.Close()
	for resultRows.Next() {
		var inserted bool
		if err := resultRows.Scan(&inserted); err != nil {
			return result, err
		}
		if inserted {
			result.Inserted++
		} else {
			result.Duplicates++
		}
	}
	return result, resultRows.Err()
}

// UsageAggregate is one pre-summed group of usage events. Tokens are summed in
// SQL (GROUP BY) so a window of tens of thousands of events collapses to a few
// hundred groups before the pricing engine runs in Go. The group key is
// everything that affects per-event pricing — model, billing_type, and whether
// the event carried a provider cost — so pricing the summed group equals
// summing per-event prices (pricing is linear per token type). Day is the
// calendar day in the user's IANA timezone.
type UsageAggregate struct {
	Agent       string
	Model       string
	Project     string
	Day         string
	BillingType string
	HasProvided bool

	InputTokens         int64
	OutputTokens        int64
	CacheCreate5mTokens int64
	CacheCreate1hTokens int64
	CacheReadTokens     int64
	ReasoningTokens     int64
	ProvidedCostUSD     float64
	EventCount          int
}

// StatsGroup converts an aggregate row into the pricing-engine group shape used
// by usagestats. Shared by the API and stats worker so both price usage events
// identically when baking AI cost into the stats cache.
func (a UsageAggregate) StatsGroup() usagestats.Group {
	return usagestats.Group{
		Agent:           a.Agent,
		Model:           a.Model,
		Project:         a.Project,
		Day:             a.Day,
		BillingType:     a.BillingType,
		HasProvided:     a.HasProvided,
		Input:           int(a.InputTokens),
		Output:          int(a.OutputTokens),
		CacheCreate5m:   int(a.CacheCreate5mTokens),
		CacheCreate1h:   int(a.CacheCreate1hTokens),
		CacheRead:       int(a.CacheReadTokens),
		Reasoning:       int(a.ReasoningTokens),
		ProvidedCostUSD: a.ProvidedCostUSD,
		EventCount:      a.EventCount,
	}
}

// UsageAggregatesBetween returns usage events in [start, end) summed into
// homogeneous pricing groups. The day is bucketed with (ts AT TIME ZONE $tz)::date
// in the user's IANA timezone (tz). A non-empty agent and/or project narrows the
// rows in SQL (agent uses the (user_id, agent, ts) index); project scopes the
// result to one project (e.g. the per-project AI cost panel).
func (s *Store) UsageAggregatesBetween(ctx context.Context, userID uuid.UUID, start, end time.Time, agent, project, tz string) ([]UsageAggregate, error) {
	if tz == "" {
		tz = "UTC"
	}
	query := `
		SELECT agent, model, coalesce(project, ''),
			to_char((ts AT TIME ZONE $4)::date, 'YYYY-MM-DD') AS day,
			coalesce(billing_type, ''),
			(cost_usd_provided IS NOT NULL) AS has_provided,
			sum(input_tokens), sum(output_tokens), sum(cache_create_5m_tokens),
			sum(cache_create_1h_tokens), sum(cache_read_tokens), sum(reasoning_tokens),
			coalesce(sum(cost_usd_provided), 0), count(*)
		FROM usage_events
		WHERE user_id = $1 AND ts >= $2 AND ts < $3`
	args := []any{userID, start, end, tz}
	if agent != "" {
		args = append(args, agent)
		query += fmt.Sprintf(" AND agent = $%d", len(args))
	}
	if project != "" {
		args = append(args, project)
		query += fmt.Sprintf(" AND coalesce(project, '') = $%d", len(args))
	}
	query += ` GROUP BY 1, 2, 3, 4, 5, 6`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aggs []UsageAggregate
	for rows.Next() {
		var a UsageAggregate
		if err := rows.Scan(&a.Agent, &a.Model, &a.Project, &a.Day, &a.BillingType, &a.HasProvided,
			&a.InputTokens, &a.OutputTokens, &a.CacheCreate5mTokens, &a.CacheCreate1hTokens,
			&a.CacheReadTokens, &a.ReasoningTokens, &a.ProvidedCostUSD, &a.EventCount); err != nil {
			return nil, err
		}
		aggs = append(aggs, a)
	}
	return aggs, rows.Err()
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
