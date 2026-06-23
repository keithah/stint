package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/keithah/stint/internal/usage"
)

// DefaultBatchSize is how many events are sent per bulk request.
const DefaultBatchSize = 500

// IngestResult is the aggregated server-reported outcome of posting events.
type IngestResult struct {
	Received   int `json:"received"`
	Inserted   int `json:"inserted"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
}

// Client posts usage events to the Stint server bulk ingest endpoint.
type Client struct {
	APIURL     string
	APIKey     string
	BatchSize  int
	HTTPClient *http.Client
}

// NewClient builds a Client with sensible defaults.
func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		APIURL:     strings.TrimRight(apiURL, "/"),
		APIKey:     apiKey,
		BatchSize:  DefaultBatchSize,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
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
	url := c.APIURL + "/users/current/usage_events.bulk"

	for start := 0; start < len(events); start += batch {
		end := start + batch
		if end > len(events) {
			end = len(events)
		}
		res, err := c.postBatch(ctx, url, events[start:end])
		if err != nil {
			return total, err
		}
		total.Received += res.Received
		total.Inserted += res.Inserted
		total.Duplicates += res.Duplicates
		total.Invalid += res.Invalid
	}
	return total, nil
}

func (c *Client) postBatch(ctx context.Context, url string, batch []usage.Event) (IngestResult, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return IngestResult{}, fmt.Errorf("marshal events: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return IngestResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return IngestResult{}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(respBody))
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return IngestResult{}, fmt.Errorf("bulk ingest failed: status %d: %s", resp.StatusCode, snippet)
	}

	var parsed bulkResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return IngestResult{}, fmt.Errorf("decode bulk response: %w", err)
	}
	return parsed.Data, nil
}
