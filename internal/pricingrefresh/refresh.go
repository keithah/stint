// Package pricingrefresh keeps a live pricing.Engine in sync with the cached
// upstream snapshots written by the weekly pricing job. Each process (API,
// worker) runs a Refresher so a fetched price update propagates without a
// redeploy; the embedded bundle remains the baseline until the first sync.
package pricingrefresh

import (
	"context"
	"time"

	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/pricing"
)

type Refresher struct {
	Store   *db.Store
	Engine  *pricing.Engine
	OnError func(error)
}

// Sync reloads the engine from the cached snapshots when either source is newer
// than `since`, returning the new high-water mark. A source with no cached
// payload is passed as nil (engine keeps its current layer for it).
func (r Refresher) Sync(ctx context.Context, since time.Time) (time.Time, error) {
	if r.Store == nil || r.Engine == nil {
		return since, nil
	}
	litellm, litAt, litOK, err := r.Store.PricingPayload(ctx, "litellm")
	if err != nil {
		return since, err
	}
	openrouter, orAt, orOK, err := r.Store.PricingPayload(ctx, "openrouter")
	if err != nil {
		return since, err
	}
	newest := litAt
	if orAt.After(newest) {
		newest = orAt
	}
	if newest.IsZero() || !newest.After(since) {
		return since, nil // nothing newer cached than what we've already applied
	}
	var litBytes, orBytes []byte
	if litOK {
		litBytes = []byte(litellm)
	}
	if orOK {
		orBytes = []byte(openrouter)
	}
	if err := r.Engine.Reload(litBytes, orBytes); err != nil {
		return since, err
	}
	return newest, nil
}

// Run syncs once immediately (so a process started after a refresh picks up the
// cached prices), then on every tick until ctx is cancelled. Errors are
// swallowed: a transient DB hiccup must not crash the host process, and the next
// tick retries.
func (r Refresher) Run(ctx context.Context, interval time.Duration) {
	since, err := r.Sync(ctx, time.Time{})
	if err != nil {
		r.reportError(err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nextSince, err := r.Sync(ctx, since)
			if err != nil {
				r.reportError(err)
				continue
			}
			since = nextSince
		}
	}
}

func (r Refresher) reportError(err error) {
	if err != nil && r.OnError != nil {
		r.OnError(err)
	}
}
