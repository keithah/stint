package stintcli

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func laterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func parseAIJSONLines(path, source string, after, fallbackTime time.Time) (aiTranscriptSummary, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return aiTranscriptSummary{}, err
	}
	defer file.Close()

	summary := aiTranscriptSummary{
		Source:      source,
		SessionID:   aiTranscriptSessionID(path),
		Files:       map[string]bool{},
		FileWrites:  map[string]bool{},
		LineChanges: map[string]int{},
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		ts := jsonTime(line)
		if ts.IsZero() {
			ts = fallbackTime
		}
		if ts.Before(after) {
			continue
		}
		if ts.After(summary.LastActivity) {
			summary.LastActivity = ts
		}
		updateAISummary(&summary, line)
	}
	if err := scanner.Err(); err != nil {
		return aiTranscriptSummary{}, err
	}
	return summary, nil
}

func aiSummaryFromJSONString(source, sessionID, raw string, after time.Time) (aiTranscriptSummary, bool) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return aiTranscriptSummary{}, false
	}
	summary := aiTranscriptSummary{
		Source:      source,
		SessionID:   first(sessionID, "unknown"),
		Files:       map[string]bool{},
		FileWrites:  map[string]bool{},
		LineChanges: map[string]int{},
	}
	updateAISummary(&summary, value)
	if summary.LastActivity.IsZero() || summary.LastActivity.Before(after) {
		return aiTranscriptSummary{}, false
	}
	return summary, true
}

func jsonTime(value map[string]any) time.Time {
	for _, key := range []string{"timestamp", "time", "created_at", "createdAt", "updated_at", "updatedAt", "startTime", "start_time", "lastInteractionTimestamp"} {
		raw, ok := value[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			if ts := parseAITimeString(v); !ts.IsZero() {
				return ts
			}
		case float64:
			if v > 1e12 {
				return time.UnixMilli(int64(v))
			}
			if v > 1e10 {
				return time.UnixMilli(int64(v))
			}
			return unixFloatTime(v)
		}
	}
	return time.Time{}
}

func parseAITimeString(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts
		}
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		if parsed > 1e10 {
			return time.UnixMilli(int64(parsed))
		}
		return unixFloatTime(parsed)
	}
	return time.Time{}
}

func jsonInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func jsonBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return parseBoolLike(v)
	default:
		return false
	}
}

func intPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
