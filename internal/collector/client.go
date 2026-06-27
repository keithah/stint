package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/keithah/stint/internal/usage"
)

// DefaultBatchSize matches the server's usage-event bulk limit so backlog
// flushes use the API's full capacity by default.
const DefaultBatchSize = 5000

// IngestResult is the aggregated server-reported outcome of posting events.
type IngestResult struct {
	Received   int `json:"received"`
	Inserted   int `json:"inserted"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
}

// Client posts usage events to the Stint server bulk ingest endpoint.
type Client struct {
	APIURL         string
	APIKey         string
	BatchSize      int
	MaxConcurrency int
	MaxRetries     int
	RetryBaseDelay time.Duration
	HTTPClient     *http.Client
}

// NewClient builds a Client with sensible defaults.
func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		APIURL:         strings.TrimRight(apiURL, "/"),
		APIKey:         apiKey,
		BatchSize:      DefaultBatchSize,
		HTTPClient:     &http.Client{Timeout: 30 * time.Second},
		MaxConcurrency: 4,
		MaxRetries:     2,
		RetryBaseDelay: 200 * time.Millisecond,
	}
}

// bulkResponse matches the server envelope: {"data":{received,inserted,...}}.
type bulkResponse struct {
	Data IngestResult `json:"data"`
}

// Post sends events in batches and returns the aggregated IngestResult. An
// empty slice is a no-op. A non-2xx response aborts with a descriptive error
// that includes the status and a snippet of the body.
func (c *Client) Post(ctx context.Context, events []usage.Event) (IngestResult, error) {
	var total IngestResult
	if len(events) == 0 {
		return total, nil
	}
	batch := c.BatchSize
	if batch <= 0 {
		batch = DefaultBatchSize
	}
	concurrency := c.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	url := c.APIURL + "/users/current/usage_events.bulk"

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for start := 0; start < len(events); start += batch {
		end := start + batch
		if end > len(events) {
			end = len(events)
		}
		batchEvents := events[start:end]
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := c.postBatch(ctx, url, batchEvents)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				return
			}
			total.Received += res.Received
			total.Inserted += res.Inserted
			total.Duplicates += res.Duplicates
			total.Invalid += res.Invalid
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return total, firstErr
	}
	return total, nil
}

func (c *Client) postBatch(ctx context.Context, url string, batch []usage.Event) (IngestResult, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return IngestResult{}, fmt.Errorf("marshal events: %w", err)
	}
	attempts := c.MaxRetries + 1
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return IngestResult{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)

		hc := c.httpClient()
		resp, err := hc.Do(req)
		if err != nil {
			lastErr = err
			if attempt+1 < attempts && shouldRetryBatch(0, err) {
				if waitErr := c.waitBeforeRetry(ctx, attempt); waitErr != nil {
					return IngestResult{}, waitErr
				}
				continue
			}
			return IngestResult{}, err
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err := bulkStatusError(resp.StatusCode, respBody)
			lastErr = err
			if attempt+1 < attempts && shouldRetryBatch(resp.StatusCode, nil) {
				if waitErr := c.waitBeforeRetry(ctx, attempt); waitErr != nil {
					return IngestResult{}, waitErr
				}
				continue
			}
			return IngestResult{}, err
		}

		var parsed bulkResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return IngestResult{}, fmt.Errorf("decode bulk response: %w", err)
		}
		return parsed.Data, nil
	}
	return IngestResult{}, lastErr
}

func bulkStatusError(status int, body []byte) error {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 500 {
		snippet = snippet[:500]
	}
	return fmt.Errorf("bulk ingest failed: status %d: %s", status, snippet)
}

func shouldRetryBatch(status int, err error) bool {
	if err != nil {
		return true
	}
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (c *Client) waitBeforeRetry(ctx context.Context, attempt int) error {
	delay := c.RetryBaseDelay
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}
