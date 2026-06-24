package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PricingSnapshot is a cached upstream price table (source "litellm" or
// "openrouter"). Payload is the raw fetched JSON; metadata drives the settings
// "last checked" display and lets every process rebuild its engine from the
// latest fetch. A row with empty Payload (status "error") records a failed fetch
// without clobbering the last good data — callers keep Payload separate.
type PricingSnapshot struct {
	Source     string    `json:"source"`
	URL        string    `json:"url"`
	Payload    string    `json:"-"`
	ModelCount int       `json:"model_count"`
	Status     string    `json:"status"`
	Error      string    `json:"error"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// ListPricingSnapshotMeta returns snapshot metadata (no payload) for display,
// ordered by source.
func (s *Store) ListPricingSnapshotMeta(ctx context.Context) ([]PricingSnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT source, url, model_count, status, error, fetched_at
		FROM pricing_snapshots
		ORDER BY source ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PricingSnapshot
	for rows.Next() {
		var p PricingSnapshot
		if err := rows.Scan(&p.Source, &p.URL, &p.ModelCount, &p.Status, &p.Error, &p.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PricingPayload returns the cached raw payload for one source (empty when never
// fetched). The latest fetched_at across sources lets the engine refresher skip
// a rebuild when nothing changed.
func (s *Store) PricingPayload(ctx context.Context, source string) (payload string, fetchedAt time.Time, ok bool, err error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT payload, fetched_at FROM pricing_snapshots
		WHERE source = $1 AND status = 'ok' AND payload <> ''`, source)
	err = row.Scan(&payload, &fetchedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", time.Time{}, false, nil
		}
		return "", time.Time{}, false, err
	}
	return payload, fetchedAt, true, nil
}

// UpsertPricingSnapshot stores a successful fetch (payload + count). modified
// fetched_at advances so refreshers detect the change.
func (s *Store) UpsertPricingSnapshot(ctx context.Context, source, url, payload string, modelCount int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO pricing_snapshots (source, url, payload, model_count, status, error, fetched_at)
		VALUES ($1, $2, $3, $4, 'ok', '', now())
		ON CONFLICT (source) DO UPDATE SET
			url = EXCLUDED.url,
			payload = EXCLUDED.payload,
			model_count = EXCLUDED.model_count,
			status = 'ok',
			error = '',
			fetched_at = now()`,
		source, url, payload, modelCount)
	return err
}

// MarkPricingSnapshotError records a failed fetch, preserving any prior good
// payload so the live engine keeps the last known prices.
func (s *Store) MarkPricingSnapshotError(ctx context.Context, source, url, message string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO pricing_snapshots (source, url, payload, model_count, status, error, fetched_at)
		VALUES ($1, $2, '', 0, 'error', $3, now())
		ON CONFLICT (source) DO UPDATE SET
			url = EXCLUDED.url,
			status = 'error',
			error = EXCLUDED.error,
			fetched_at = now()`,
		source, url, message)
	return err
}
