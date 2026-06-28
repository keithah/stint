package stintcli

import (
	"fmt"
	"math"
	"time"
)

const (
	backoffFactorSeconds = 15
	backoffMaxSeconds    = 3600
	wakaTimeDateFormat   = time.RFC3339
)

func parseBackoffAt(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(wakaTimeDateFormat, value)
	if err != nil {
		return time.Time{}
	}
	if t.After(time.Now()) {
		return time.Now()
	}
	return t
}

func shouldBackoffNow(retries int, at time.Time, now time.Time) bool {
	if retries < 1 || at.IsZero() {
		return false
	}
	backoffSeconds := float64(backoffFactorSeconds) * math.Pow(2, float64(retries))
	if backoffSeconds > backoffMaxSeconds {
		return false
	}
	return now.Before(at.Add(time.Duration(backoffSeconds) * time.Second))
}

func (o Options) shouldBackoff() bool {
	return shouldBackoffNow(o.BackoffRetries, o.BackoffAt, time.Now())
}

func (o Options) recordBackoffFailure() error {
	return writeBackoffState(o.InternalConfigPath, o.BackoffRetries+1, time.Now())
}

func (o Options) resetBackoff() error {
	if o.BackoffRetries < 1 && o.BackoffAt.IsZero() {
		return nil
	}
	return writeBackoffState(o.InternalConfigPath, 0, time.Time{})
}

func writeBackoffState(path string, retries int, at time.Time) error {
	atText := ""
	if !at.IsZero() {
		atText = at.Format(wakaTimeDateFormat)
	}
	if err := WriteConfigValue(path, "internal", "backoff_retries", fmt.Sprintf("%d", retries)); err != nil {
		return err
	}
	return WriteConfigValue(path, "internal", "backoff_at", atText)
}
