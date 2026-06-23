package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CustomPricing is a user-defined per-model price for private/proxied/unpriced
// models. Prices are per-million tokens (human-friendly); callers convert to
// per-token when feeding the pricing engine. Mirrors LiteLLM custom prices.
type CustomPricing struct {
	Model                   string    `json:"model"`
	InputPerMillionUSD      float64   `json:"input_per_million_usd"`
	OutputPerMillionUSD     float64   `json:"output_per_million_usd"`
	CacheWritePerMillionUSD float64   `json:"cache_write_per_million_usd"`
	CacheReadPerMillionUSD  float64   `json:"cache_read_per_million_usd"`
	CreatedAt               time.Time `json:"created_at"`
	ModifiedAt              time.Time `json:"modified_at"`
}

// ValidateCustomPricing enforces a non-empty model and non-negative prices.
func ValidateCustomPricing(p CustomPricing) error {
	if strings.TrimSpace(p.Model) == "" {
		return errors.New("model is required")
	}
	if p.InputPerMillionUSD < 0 || p.OutputPerMillionUSD < 0 ||
		p.CacheWritePerMillionUSD < 0 || p.CacheReadPerMillionUSD < 0 {
		return errors.New("custom pricing must be non-negative")
	}
	return nil
}

// ListCustomPricing returns the user's custom model prices ordered by model.
func (s *Store) ListCustomPricing(ctx context.Context, userID uuid.UUID) ([]CustomPricing, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT model, input_per_million_usd, output_per_million_usd,
			cache_write_per_million_usd, cache_read_per_million_usd, created_at, modified_at
		FROM custom_pricing
		WHERE user_id = $1
		ORDER BY model ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prices []CustomPricing
	for rows.Next() {
		var p CustomPricing
		if err := rows.Scan(&p.Model, &p.InputPerMillionUSD, &p.OutputPerMillionUSD,
			&p.CacheWritePerMillionUSD, &p.CacheReadPerMillionUSD, &p.CreatedAt, &p.ModifiedAt); err != nil {
			return nil, err
		}
		prices = append(prices, p)
	}
	return prices, rows.Err()
}

// UpsertCustomPricing inserts or updates one custom model price for the user.
func (s *Store) UpsertCustomPricing(ctx context.Context, userID uuid.UUID, p CustomPricing) error {
	if err := ValidateCustomPricing(p); err != nil {
		return err
	}
	model := strings.TrimSpace(p.Model)
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO custom_pricing (
			user_id, model, input_per_million_usd, output_per_million_usd,
			cache_write_per_million_usd, cache_read_per_million_usd, modified_at
		) VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (user_id, model) DO UPDATE SET
			input_per_million_usd = EXCLUDED.input_per_million_usd,
			output_per_million_usd = EXCLUDED.output_per_million_usd,
			cache_write_per_million_usd = EXCLUDED.cache_write_per_million_usd,
			cache_read_per_million_usd = EXCLUDED.cache_read_per_million_usd,
			modified_at = now()`,
		userID, model, p.InputPerMillionUSD, p.OutputPerMillionUSD,
		p.CacheWritePerMillionUSD, p.CacheReadPerMillionUSD)
	return err
}

// DeleteCustomPricing removes one custom model price for the user.
func (s *Store) DeleteCustomPricing(ctx context.Context, userID uuid.UUID, model string) error {
	_, err := s.Pool.Exec(ctx, `
		DELETE FROM custom_pricing WHERE user_id = $1 AND model = $2`,
		userID, strings.TrimSpace(model))
	return err
}
