package stintcli

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const aiCodingCategory = "ai coding"

var antigravityCodeActionPathRe = regexp.MustCompile(`(?s)tool to:\s*(.+?)\.\s+If relevant`)

var claudeSystemReminderRe = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

type aiTranscriptSource struct {
	Name       string
	Root       string
	Extensions []string
}

type aiTranscriptSummary struct {
	Agent            string
	AgentVersion     string
	Source           string
	SessionID        string
	CWD              string
	LastActivity     time.Time
	InputTokens      int
	Model            string
	OutputTokens     int
	Provider         string
	PromptLength     int
	SubscriptionPlan string
	Files            map[string]bool
	FileEvents       []aiFileEvent
	FileWrites       map[string]bool
	LineChanges      map[string]int
	PendingPatch     map[string]string
}

type aiFileEvent struct {
	Path        string
	IsWrite     bool
	LineChanges *int
}

func runSyncAIActivity(stdout anyWriter, opts Options) error {
	heartbeats, lastActivity, err := collectAIHeartbeats(opts)
	if err != nil {
		return err
	}
	if len(heartbeats) == 0 {
		fmt.Fprintln(stdout, "synced=0")
		return nil
	}
	if err := sendHeartbeats(stdout, opts, heartbeats, "", false, false); err != nil {
		return err
	}
	_ = recordAISyncAfter(opts, lastActivity)
	fmt.Fprintf(stdout, "ai_heartbeats=%d\n", len(heartbeats))
	return nil
}

type anyWriter interface {
	Write([]byte) (int, error)
}

func collectAIHeartbeats(opts Options) ([]Heartbeat, time.Time, error) {
	after := aiSyncAfter(opts)
	sources := aiSources()
	var heartbeats []Heartbeat
	var lastActivity time.Time
	for _, source := range sources {
		summaries, err := parseAITranscriptSource(source, after)
		if err != nil {
			return nil, time.Time{}, err
		}
		for _, summary := range summaries {
			if summary.LastActivity.After(lastActivity) {
				lastActivity = summary.LastActivity
			}
			heartbeats = append(heartbeats, aiSummaryHeartbeats(summary, opts)...)
		}
	}
	sqliteSummaries, err := collectAISQLiteSummaries(after)
	if err != nil {
		return nil, time.Time{}, err
	}
	for _, summary := range sqliteSummaries {
		if summary.LastActivity.After(lastActivity) {
			lastActivity = summary.LastActivity
		}
		heartbeats = append(heartbeats, aiSummaryHeartbeats(summary, opts)...)
	}
	heartbeats = filterAIHeartbeats(heartbeats, opts)
	sort.SliceStable(heartbeats, func(i, j int) bool {
		if heartbeats[i].Time == heartbeats[j].Time {
			return heartbeats[i].Entity < heartbeats[j].Entity
		}
		return heartbeats[i].Time < heartbeats[j].Time
	})
	return heartbeats, lastActivity, nil
}

func filterAIHeartbeats(heartbeats []Heartbeat, opts Options) []Heartbeat {
	if len(opts.Include) == 0 && len(opts.Exclude) == 0 {
		return heartbeats
	}
	keptFilesBySession := map[string]bool{}
	fileSeenBySession := map[string]bool{}
	kept := make([]Heartbeat, 0, len(heartbeats))
	for _, hb := range heartbeats {
		if hb.EntityType != "file" {
			continue
		}
		fileSeenBySession[hb.AISession] = true
		skip, err := excluded(hb.Entity, opts.Include, opts.Exclude)
		if err != nil || skip {
			continue
		}
		keptFilesBySession[hb.AISession] = true
		kept = append(kept, hb)
	}
	for _, hb := range heartbeats {
		if hb.EntityType == "file" {
			continue
		}
		if fileSeenBySession[hb.AISession] {
			if keptFilesBySession[hb.AISession] {
				kept = append(kept, hb)
			}
			continue
		}
		skip, err := excluded(hb.Entity, opts.Include, opts.Exclude)
		if err != nil || skip {
			continue
		}
		kept = append(kept, hb)
	}
	return kept
}

func recordAISyncAfter(opts Options, lastActivity time.Time) error {
	if lastActivity.IsZero() {
		return nil
	}
	return WriteConfigValues(opts.InternalConfigPath, "internal", map[string]string{
		"ai_logs_last_parsed_at": lastActivity.UTC().Format(time.RFC3339),
		"ai_sync_after":          fmt.Sprintf("%.6f", float64(lastActivity.UnixNano())/1e9),
	})
}

func aiSyncAfter(opts Options) time.Time {
	if opts.SyncAIAfter > 0 {
		return unixFloatTime(opts.SyncAIAfter)
	}
	if raw := strings.TrimSpace(opts.InternalConfig.Get("internal", "ai_logs_last_parsed_at")); raw != "" {
		if parsed, ok := parseAISyncAfter(raw); ok {
			return parsed
		}
	}
	if raw := strings.TrimSpace(opts.InternalConfig.Get("internal", "ai_sync_after")); raw != "" {
		if parsed, ok := parseAISyncAfter(raw); ok {
			return parsed
		}
	}
	return time.Now().Add(-24 * time.Hour)
}

func parseAISyncAfter(raw string) (time.Time, bool) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil && !parsed.IsZero() {
		return parsed, true
	}
	if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
		return unixFloatTime(parsed), true
	}
	return time.Time{}, false
}

func unixFloatTime(seconds float64) time.Time {
	whole, frac := math.Modf(seconds)
	return time.Unix(int64(whole), int64(frac*1e9))
}

var aiPathRe = regexp.MustCompile(`^[A-Za-z0-9_./~:\\ -]+\.[A-Za-z0-9]{1,12}$`)
