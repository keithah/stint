package db

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// BillingPref is a user-defined per-agent billing-mode override. It declares
// whether an agent's usage is flat-rate subscription (zero marginal cost,
// covered by the subscription) or metered API (marginal = equivalent-API cost),
// overriding the billing_type the collecting adapter stamped on stored events.
type BillingPref struct {
	Agent       string `json:"agent"`
	BillingType string `json:"billing_type"`
}

// ValidateBillingPref enforces a non-empty agent and a billing_type that is one
// of the two recognized modes.
func ValidateBillingPref(p BillingPref) error {
	if strings.TrimSpace(p.Agent) == "" {
		return errors.New("agent is required")
	}
	switch p.BillingType {
	case "api", "subscription":
		return nil
	default:
		return errors.New("billing_type must be 'api' or 'subscription'")
	}
}

// ListBillingPrefs returns the user's per-agent billing overrides ordered by agent.
func (s *Store) ListBillingPrefs(ctx context.Context, userID uuid.UUID) ([]BillingPref, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT agent, billing_type
		FROM billing_prefs
		WHERE user_id = $1
		ORDER BY agent ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prefs []BillingPref
	for rows.Next() {
		var p BillingPref
		if err := rows.Scan(&p.Agent, &p.BillingType); err != nil {
			return nil, err
		}
		prefs = append(prefs, p)
	}
	return prefs, rows.Err()
}

// UpsertBillingPref inserts or updates one per-agent billing override.
func (s *Store) UpsertBillingPref(ctx context.Context, userID uuid.UUID, p BillingPref) error {
	if err := ValidateBillingPref(p); err != nil {
		return err
	}
	agent := strings.TrimSpace(p.Agent)
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO billing_prefs (user_id, agent, billing_type, modified_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id, agent) DO UPDATE SET
			billing_type = EXCLUDED.billing_type,
			modified_at = now()`,
		userID, agent, p.BillingType)
	return err
}

// DeleteBillingPref removes one per-agent billing override.
func (s *Store) DeleteBillingPref(ctx context.Context, userID uuid.UUID, agent string) error {
	_, err := s.Pool.Exec(ctx, `
		DELETE FROM billing_prefs WHERE user_id = $1 AND agent = $2`,
		userID, strings.TrimSpace(agent))
	return err
}
