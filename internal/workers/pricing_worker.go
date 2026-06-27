package workers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/pricing"
)

// PricingWorker refreshes the cached upstream price tables. It fetches each
// source, validates by parsing, and stores the raw payload; the per-process
// engine refresher then reloads the live engine from the cache. A failed fetch
// is recorded without discarding the last good payload.
type PricingWorker struct {
	Store  *db.Store
	Client *http.Client
}

func (w PricingWorker) HandlePricingRefreshTask(ctx context.Context, _ *asynq.Task) error {
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	// Both sources are refreshed independently; one failing must not block the
	// other. Errors are recorded per source and the task still succeeds so the
	// scheduler does not retry-storm on a flaky upstream.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		w.refresh(ctx, client, "litellm", pricing.LiteLLMURL, pricing.FetchLiteLLM, pricing.CountLiteLLM)
	}()
	go func() {
		defer wg.Done()
		w.refresh(ctx, client, "openrouter", pricing.OpenRouterURL, pricing.FetchOpenRouter, pricing.CountOpenRouter)
	}()
	wg.Wait()
	return nil
}

func (w PricingWorker) refresh(
	ctx context.Context,
	client *http.Client,
	source, url string,
	fetch func(context.Context, *http.Client) ([]byte, error),
	count func([]byte) (int, error),
) {
	data, err := fetch(ctx, client)
	if err != nil {
		_ = w.Store.MarkPricingSnapshotError(ctx, source, url, err.Error())
		return
	}
	n, err := count(data)
	if err != nil {
		_ = w.Store.MarkPricingSnapshotError(ctx, source, url, "parse: "+err.Error())
		return
	}
	_ = w.Store.UpsertPricingSnapshot(ctx, source, url, string(data), n)
}
