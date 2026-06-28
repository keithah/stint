package stintcli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
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

func aiSources() []aiTranscriptSource {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	sources := []aiTranscriptSource{
		{Name: "Codex", Root: filepath.Join(home, ".codex", "sessions"), Extensions: []string{".jsonl"}},
		{Name: "Claude", Root: filepath.Join(home, ".claude", "projects"), Extensions: []string{".jsonl"}},
		{Name: "Continue", Root: filepath.Join(home, ".continue", "dev_data"), Extensions: []string{".jsonl"}},
		{Name: "Amp", Root: filepath.Join(home, ".cache", "amp", "logs", "threads"), Extensions: []string{".log"}},
		{Name: "Copilot", Root: filepath.Join(home, ".copilot", "session-state"), Extensions: []string{".jsonl"}},
		{Name: "Copilot", Root: filepath.Join(home, "Library", "Application Support", "Code", "User", "workspaceStorage"), Extensions: []string{".json"}},
		{Name: "Copilot", Root: filepath.Join(home, "AppData", "Roaming", "Code", "User", "workspaceStorage"), Extensions: []string{".json"}},
		{Name: "Copilot", Root: filepath.Join(home, ".config", "Code", "User", "workspaceStorage"), Extensions: []string{".json"}},
		{Name: "Copilot", Root: filepath.Join(home, ".config", "code", "User", "workspaceStorage"), Extensions: []string{".json"}},
		{Name: "Gemini", Root: filepath.Join(home, ".gemini", "tmp"), Extensions: []string{".json"}},
		{Name: "Antigravity Desktop", Root: filepath.Join(home, ".gemini", "antigravity"), Extensions: []string{".json", ".jsonl"}},
		{Name: "Antigravity IDE", Root: filepath.Join(home, ".gemini", "antigravity-ide"), Extensions: []string{".json", ".jsonl"}},
		{Name: "Antigravity CLI", Root: filepath.Join(home, ".gemini", "antigravity-cli"), Extensions: []string{".json", ".jsonl"}},
		{Name: "Pi", Root: filepath.Join(home, ".pi", "agent", "sessions"), Extensions: []string{".jsonl"}},
		{Name: "Qoder", Root: filepath.Join(home, ".qoder", "cache", "projects"), Extensions: []string{".jsonl"}},
		{Name: "Qwen Code", Root: filepath.Join(home, ".qwen", "projects"), Extensions: []string{".jsonl"}},
		{Name: "OpenCode", Root: filepath.Join(home, ".local", "share", "opencode", "storage"), Extensions: []string{".json"}},
		{Name: "OpenCode", Root: filepath.Join(home, "Library", "Application Support", "opencode", "storage"), Extensions: []string{".json"}},
		{Name: "OpenCode", Root: filepath.Join(home, "AppData", "Local", "opencode", "storage"), Extensions: []string{".json"}},
		{Name: "OpenCode", Root: filepath.Join(home, "AppData", "Roaming", "opencode", "storage"), Extensions: []string{".json"}},
		{Name: "Kiro", Root: filepath.Join(home, ".config", "Kiro", "User", "globalStorage", "kiro.kiroagent"), Extensions: []string{".json"}},
		{Name: "Kiro", Root: filepath.Join(home, ".config", "kiro", "User", "globalStorage", "kiro.kiroagent"), Extensions: []string{".json"}},
		{Name: "Kiro", Root: filepath.Join(home, "Library", "Application Support", "Kiro", "User", "globalStorage", "kiro.kiroagent"), Extensions: []string{".json"}},
		{Name: "Kiro", Root: filepath.Join(home, "AppData", "Roaming", "Kiro", "User", "globalStorage", "kiro.kiroagent"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, ".config", "Windsurf", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, ".vscode-server", "data", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, ".cursor-server", "data", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, "AppData", "Roaming", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Cline", Root: filepath.Join(home, "AppData", "Roaming", "Cursor", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, ".config", "Code", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, ".config", "Code", "User", "globalStorage", "RooVeterinaryInc.roo-code-nightly", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, ".vscode-server", "data", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, ".cursor-server", "data", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, "AppData", "Roaming", "Code", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, "AppData", "Roaming", "Cursor", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, "AppData", "Roaming", "Code", "User", "globalStorage", "RooVeterinaryInc.roo-code-nightly", "tasks"), Extensions: []string{".json"}},
		{Name: "Roo Code", Root: filepath.Join(home, ".vscode-server", "data", "User", "globalStorage", "RooVeterinaryInc.roo-code-nightly", "tasks"), Extensions: []string{".json"}},
		{Name: "Cody", Root: filepath.Join(home, ".config", "Code", "User", "globalStorage", "sourcegraph.cody-ai"), Extensions: []string{".json"}},
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		sources = append(sources, aiTranscriptSource{Name: "Amp", Root: filepath.Join(cacheDir, "amp", "logs", "threads"), Extensions: []string{".log"}})
	}
	if runtimeDir := strings.TrimSpace(os.Getenv("QWEN_RUNTIME_DIR")); runtimeDir != "" {
		sources = append(sources, aiTranscriptSource{Name: "Qwen Code", Root: filepath.Join(expandHome(runtimeDir), "projects"), Extensions: []string{".jsonl"}})
	}
	if qwenHome := strings.TrimSpace(os.Getenv("QWEN_HOME")); qwenHome != "" {
		sources = append(sources, aiTranscriptSource{Name: "Qwen Code", Root: filepath.Join(expandHome(qwenHome), "projects"), Extensions: []string{".jsonl"}})
	}
	deduped := make([]aiTranscriptSource, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		key := source.Name + "\x00" + filepath.Clean(source.Root)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, source)
	}
	return deduped
}

func parseAITranscriptSource(source aiTranscriptSource, after time.Time) ([]aiTranscriptSummary, error) {
	if source.Root == "" {
		return nil, nil
	}
	if info, err := os.Stat(source.Root); err != nil || !info.IsDir() {
		return nil, nil
	}
	if source.Name == "Continue" {
		return parseContinueAITranscripts(source.Root, after)
	}
	if source.Name == "Kiro" {
		return parseKiroAITranscripts(source.Root, after)
	}
	var summaries []aiTranscriptSummary
	err := filepath.WalkDir(source.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !source.matches(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(after) {
			return nil
		}
		summary, err := parseAITranscriptFile(path, source.Name, after, info.ModTime())
		if err != nil {
			return err
		}
		if !summary.LastActivity.IsZero() {
			summaries = append(summaries, summary)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s transcripts: %w", source.Name, err)
	}
	return summaries, nil
}

type continueAIEvent struct {
	Kind         string
	Time         time.Time
	SessionID    string
	Workspace    string
	FilePath     string
	IsWrite      bool
	LineChanges  *int
	Model        string
	Provider     string
	Prompt       string
	InputTokens  int
	OutputTokens int
}

func parseContinueAITranscripts(root string, after time.Time) ([]aiTranscriptSummary, error) {
	workspaces := continueSessionWorkspaces(filepath.Join(filepath.Dir(root), "sessions", "sessions.json"))
	events, err := continueAIEvents(root, workspaces)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	tokenEvents := continueTokenEvents(events)
	summaries := map[string]*aiTranscriptSummary{}
	var currentSessionID, currentWorkspace string
	for _, event := range events {
		if event.SessionID != "" {
			currentSessionID = event.SessionID
		}
		if event.Workspace != "" {
			currentWorkspace = event.Workspace
		}
		if event.Time.Before(after) {
			continue
		}
		switch event.Kind {
		case "tokensGenerated":
			continue
		case "chatInteraction":
			summary := continueSummary(summaries, event.SessionID)
			summary.CWD = first(event.Workspace, summary.CWD)
			summary.LastActivity = laterTime(summary.LastActivity, event.Time)
			summary.Model = first(event.Model, summary.Model)
			summary.Provider = first(event.Provider, summary.Provider)
			if prompt := continuePromptText(event.Prompt); prompt != "" && summary.PromptLength == 0 {
				summary.PromptLength = len([]rune(prompt))
			}
			if tokens, ok := matchContinueTokens(event, tokenEvents); ok {
				summary.InputTokens += tokens.InputTokens
				summary.OutputTokens += tokens.OutputTokens
			}
		case "toolUsage", "editOutcome":
			sessionID := first(event.SessionID, currentSessionID)
			summary := continueSummary(summaries, sessionID)
			workspace := first(event.Workspace, currentWorkspace, summary.CWD)
			summary.CWD = first(workspace, summary.CWD)
			summary.LastActivity = laterTime(summary.LastActivity, event.Time)
			summary.Model = first(event.Model, summary.Model)
			summary.Provider = first(event.Provider, summary.Provider)
			if prompt := continuePromptText(event.Prompt); prompt != "" && summary.PromptLength == 0 {
				summary.PromptLength = len([]rune(prompt))
			}
			path := normalizeAIPath(event.FilePath, workspace)
			if path == "" {
				continue
			}
			summary.Files[path] = true
			summary.FileWrites[path] = event.IsWrite
			if event.LineChanges != nil {
				summary.LineChanges[path] = *event.LineChanges
			}
		}
	}
	out := make([]aiTranscriptSummary, 0, len(summaries))
	for _, summary := range summaries {
		if !summary.LastActivity.IsZero() {
			out = append(out, *summary)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}

func continueAIEvents(root string, workspaces map[string]string) ([]continueAIEvent, error) {
	var events []continueAIEvent
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".jsonl" {
			return nil
		}
		parsed, err := continueAIEventsFromFile(path, workspaces)
		if err != nil {
			return err
		}
		events = append(events, parsed...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk Continue transcripts: %w", err)
	}
	return events, nil
}

func continueAIEventsFromFile(path string, workspaces map[string]string) ([]continueAIEvent, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []continueAIEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var payload map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &payload); err != nil {
			continue
		}
		event, ok := continueAIEventFromPayload(payload, workspaces)
		if ok {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func continueAIEventFromPayload(payload map[string]any, workspaces map[string]string) (continueAIEvent, bool) {
	kind := firstString(payload["eventName"])
	if kind == "" {
		return continueAIEvent{}, false
	}
	event := continueAIEvent{
		Kind:         kind,
		Time:         jsonTime(payload),
		SessionID:    firstString(payload["sessionId"]),
		Model:        firstString(payload["modelName"], payload["model"]),
		Provider:     firstString(payload["modelProvider"], payload["provider"]),
		Prompt:       firstString(payload["prompt"]),
		InputTokens:  jsonInt(payload["promptTokens"]),
		OutputTokens: firstNonZeroInt(jsonInt(payload["generatedTokens"]), jsonInt(payload["completionTokens"]), jsonInt(payload["outputTokens"])),
	}
	if event.SessionID != "" {
		event.Workspace = workspaces[event.SessionID]
	}
	switch kind {
	case "tokensGenerated", "chatInteraction":
		return event, !event.Time.IsZero()
	case "toolUsage":
		if payload["accepted"] == false || payload["succeeded"] == false {
			return continueAIEvent{}, false
		}
		event.FilePath = continueToolFilePath(payload["toolCallArgs"])
		event.IsWrite = !strings.Contains(strings.ToLower(firstString(payload["functionName"])), "read")
		lineChanges := 0
		event.LineChanges = &lineChanges
		return event, !event.Time.IsZero() && event.FilePath != ""
	case "editOutcome":
		if payload["accepted"] == false {
			return continueAIEvent{}, false
		}
		event.FilePath = firstString(payload["filepath"], payload["filePath"])
		event.IsWrite = true
		lineChanges := jsonInt(payload["lineChange"])
		event.LineChanges = &lineChanges
		return event, !event.Time.IsZero() && event.FilePath != ""
	default:
		return continueAIEvent{}, false
	}
}

func continueSessionWorkspaces(path string) map[string]string {
	workspaces := map[string]string{}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return workspaces
	}
	var sessions []map[string]any
	if err := json.Unmarshal(data, &sessions); err != nil {
		return workspaces
	}
	for _, session := range sessions {
		sessionID := firstString(session["sessionId"], session["sessionID"])
		workspace := normalizeAIWorkspacePath(firstString(session["workspaceDirectory"], session["workspace"]))
		if sessionID != "" && workspace != "" {
			workspaces[sessionID] = workspace
		}
	}
	return workspaces
}

func normalizeAIWorkspacePath(value string) string {
	value = strings.TrimSpace(normalizeAIFileURI(value))
	if value == "" {
		return ""
	}
	value = expandHome(value)
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return filepath.Clean(value)
}

func continueToolFilePath(raw any) string {
	var payload any
	switch value := raw.(type) {
	case string:
		if parsed, ok := parseAIEmbeddedJSON(value); ok {
			payload = parsed
		}
	case map[string]any:
		payload = value
	}
	if payload == nil {
		return ""
	}
	if object, ok := payload.(map[string]any); ok {
		return firstString(object["filepath"], object["filePath"], object["path"])
	}
	return ""
}

func continueSummary(summaries map[string]*aiTranscriptSummary, sessionID string) *aiTranscriptSummary {
	sessionID = first(sessionID, "unknown")
	if summary, ok := summaries[sessionID]; ok {
		return summary
	}
	summary := &aiTranscriptSummary{
		Source:      "Continue",
		SessionID:   sessionID,
		Files:       map[string]bool{},
		FileWrites:  map[string]bool{},
		LineChanges: map[string]int{},
	}
	summaries[sessionID] = summary
	return summary
}

func continueTokenEvents(events []continueAIEvent) []continueAIEvent {
	var tokens []continueAIEvent
	for _, event := range events {
		if event.Kind == "tokensGenerated" {
			tokens = append(tokens, event)
		}
	}
	return tokens
}

func matchContinueTokens(chat continueAIEvent, tokens []continueAIEvent) (continueAIEvent, bool) {
	var best continueAIEvent
	var bestDiff time.Duration
	for _, token := range tokens {
		if chat.Model != "" && token.Model != "" && !strings.EqualFold(chat.Model, token.Model) {
			continue
		}
		if chat.Provider != "" && token.Provider != "" && !strings.EqualFold(chat.Provider, token.Provider) {
			continue
		}
		diff := chat.Time.Sub(token.Time)
		if diff < 0 {
			diff = -diff
		}
		if diff > 5*time.Second {
			continue
		}
		if best.Time.IsZero() || diff < bestDiff {
			best = token
			bestDiff = diff
		}
	}
	return best, !best.Time.IsZero()
}

func continuePromptText(prompt string) string {
	if idx := strings.LastIndex(prompt, "<user>"); idx >= 0 {
		prompt = prompt[idx+len("<user>"):]
	}
	if idx := strings.Index(prompt, "<"); idx >= 0 {
		prompt = prompt[:idx]
	}
	return strings.TrimSpace(prompt)
}

func laterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

type kiroSessionInfo struct {
	SessionID    string
	Workspace    string
	PromptLength int
}

func parseKiroAITranscripts(root string, after time.Time) ([]aiTranscriptSummary, error) {
	sessions, err := kiroSessions(root)
	if err != nil {
		return nil, err
	}
	summaries := map[string]*aiTranscriptSummary{}
	for sessionID, session := range sessions {
		summaries[sessionID] = &aiTranscriptSummary{
			Source:       "Kiro",
			SessionID:    sessionID,
			CWD:          session.Workspace,
			PromptLength: session.PromptLength,
			Files:        map[string]bool{},
			FileWrites:   map[string]bool{},
			LineChanges:  map[string]int{},
		}
	}
	if err := kiroApplyExecutions(root, sessions, summaries, after); err != nil {
		return nil, err
	}
	var out []aiTranscriptSummary
	for _, summary := range summaries {
		if !summary.LastActivity.IsZero() && !summary.LastActivity.Before(after) {
			out = append(out, *summary)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}

func kiroSessions(root string) (map[string]kiroSessionInfo, error) {
	sessions := map[string]kiroSessionInfo{}
	sessionRoot := filepath.Join(root, "workspace-sessions")
	if info, err := os.Stat(sessionRoot); err != nil || !info.IsDir() {
		return sessions, nil
	}
	err := filepath.WalkDir(sessionRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			return nil
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		var session map[string]any
		if err := json.Unmarshal(data, &session); err != nil {
			return nil
		}
		sessionID := firstString(session["sessionId"], session["sessionID"])
		if sessionID == "" {
			return nil
		}
		workspace := normalizeAIWorkspacePath(firstString(session["workspacePath"], session["workspaceDirectory"]))
		sessions[sessionID] = kiroSessionInfo{
			SessionID:    sessionID,
			Workspace:    workspace,
			PromptLength: len([]rune(kiroSessionPrompt(session))),
		}
		return nil
	})
	return sessions, err
}

func kiroSessionPrompt(session map[string]any) string {
	history, _ := session["history"].([]any)
	for i := len(history) - 1; i >= 0; i-- {
		item, _ := history[i].(map[string]any)
		message, _ := item["message"].(map[string]any)
		if !strings.EqualFold(firstString(message["role"]), "user") {
			continue
		}
		if text := aiTextContent(message["content"]); text != "" {
			return text
		}
	}
	return ""
}

func aiTextContent(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			object, _ := item.(map[string]any)
			if text := firstString(object["text"]); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	default:
		return ""
	}
}

func kiroApplyExecutions(root string, sessions map[string]kiroSessionInfo, summaries map[string]*aiTranscriptSummary, after time.Time) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.Contains(path, string(filepath.Separator)+"workspace-sessions"+string(filepath.Separator)) {
			return nil
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		var execution map[string]any
		if err := json.Unmarshal(data, &execution); err != nil {
			return nil
		}
		sessionID := firstString(execution["chatSessionId"], execution["sessionId"])
		if sessionID == "" {
			return nil
		}
		summary := summaries[sessionID]
		if summary == nil {
			session := sessions[sessionID]
			summary = &aiTranscriptSummary{
				Source:      "Kiro",
				SessionID:   sessionID,
				CWD:         session.Workspace,
				Files:       map[string]bool{},
				FileWrites:  map[string]bool{},
				LineChanges: map[string]int{},
			}
			summaries[sessionID] = summary
		}
		actions, _ := execution["actions"].([]any)
		for _, rawAction := range actions {
			action, _ := rawAction.(map[string]any)
			if !strings.EqualFold(firstString(action["actionState"]), "Accepted") {
				continue
			}
			ts := kiroActionTime(action)
			if ts.IsZero() || ts.Before(after) {
				continue
			}
			if ts.After(summary.LastActivity) {
				summary.LastActivity = ts
			}
			kiroApplyAction(summary, action)
		}
		return nil
	})
}

func kiroActionTime(action map[string]any) time.Time {
	value := jsonInt(action["emittedAt"])
	if value == 0 {
		value = jsonInt(action["startTime"])
	}
	if value > 0 {
		return time.UnixMilli(int64(value))
	}
	return time.Time{}
}

func kiroApplyAction(summary *aiTranscriptSummary, action map[string]any) {
	input, _ := action["input"].(map[string]any)
	switch strings.ToLower(firstString(action["actionType"])) {
	case "readfiles":
		files, _ := input["files"].([]any)
		for _, rawFile := range files {
			file, _ := rawFile.(map[string]any)
			path := normalizeKiroPath(firstString(file["path"]), summary.CWD)
			if path != "" {
				summary.FileEvents = append(summary.FileEvents, aiFileEvent{Path: path, IsWrite: false})
			}
		}
	case "replace":
		path := normalizeKiroPath(firstString(input["local"], input["file"]), summary.CWD)
		if path == "" {
			return
		}
		lineChanges := countAIContentLines(firstString(input["modifiedContent"])) - countAIContentLines(firstString(input["originalContent"]))
		summary.FileEvents = append(summary.FileEvents, aiFileEvent{Path: path, IsWrite: true, LineChanges: &lineChanges})
	}
}

func normalizeKiroPath(value, cwd string) string {
	value = strings.TrimSpace(normalizeAIFileURI(value))
	if value == "" || strings.Contains(value, "\n") {
		return ""
	}
	value = expandHome(value)
	if !filepath.IsAbs(value) && cwd != "" {
		value = filepath.Join(cwd, value)
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return filepath.Clean(value)
}

func (source aiTranscriptSource) matches(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, allowed := range source.Extensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

func parseAITranscriptFile(path, source string, after, fallbackTime time.Time) (aiTranscriptSummary, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return parseAIJSONFile(path, source, after, fallbackTime)
	default:
		return parseAIJSONLines(path, source, after, fallbackTime)
	}
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

func parseAIJSONFile(path, source string, after, fallbackTime time.Time) (aiTranscriptSummary, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return aiTranscriptSummary{}, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return aiTranscriptSummary{}, err
	}
	summary := aiTranscriptSummary{
		Source:       source,
		SessionID:    aiTranscriptSessionID(path),
		CWD:          aiTranscriptProjectRoot(path, source),
		LastActivity: fallbackTime,
		Files:        map[string]bool{},
		FileWrites:   map[string]bool{},
		LineChanges:  map[string]int{},
	}
	updateAISummary(&summary, value)
	if summary.LastActivity.Before(after) {
		return aiTranscriptSummary{}, nil
	}
	return summary, nil
}

func aiTranscriptProjectRoot(path, source string) string {
	if source != "Gemini" {
		return ""
	}
	for dir := filepath.Dir(path); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		rootPath := filepath.Join(dir, ".project_root")
		data, err := os.ReadFile(filepath.Clean(rootPath))
		if err == nil {
			return normalizeAIWorkspacePath(string(data))
		}
		if filepath.Base(dir) == "tmp" {
			break
		}
	}
	return ""
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

func updateAISummary(summary *aiTranscriptSummary, value any) {
	switch v := value.(type) {
	case map[string]any:
		if ts := jsonTime(v); !ts.IsZero() && ts.After(summary.LastActivity) {
			summary.LastActivity = ts
		}
		updateAICopilotCLIMetadata(summary, v)
		for key, raw := range v {
			lower := strings.ToLower(key)
			if lower == "sessionid" || lower == "session_id" || lower == "chatsessionid" || lower == "chat_session_id" || lower == "conversationid" || lower == "conversation_id" || lower == "threadid" || lower == "thread_id" || lower == "id" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					if lower != "id" || summary.SessionID == "" {
						summary.SessionID = strings.TrimSpace(s)
					}
				}
			}
			if lower == "cwd" || lower == "workspace" || lower == "workspacedirectory" || lower == "workspace_directory" || lower == "workspaceroot" || lower == "workspace_root" || lower == "workdir" || lower == "workingdir" || lower == "working_dir" || lower == "projectpath" || lower == "project_path" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.CWD = expandHome(strings.TrimSpace(s))
				}
			}
			if lower == "model" || lower == "modelname" || lower == "model_name" || lower == "modelid" || lower == "model_id" || lower == "modelslug" || lower == "selectedmodel" || lower == "selectedchatmodel" || lower == "newmodel" || lower == "currentmodel" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.Model = strings.TrimSpace(s)
				}
			}
			if lower == "provider" || lower == "modelprovider" || lower == "model_provider" || lower == "aiprovider" || lower == "ai_provider" || lower == "llmprovider" || lower == "llm_provider" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.Provider = strings.TrimSpace(s)
				}
			}
			if lower == "subscriptionplan" || lower == "subscription_plan" || lower == "aisubscriptionplan" || lower == "ai_subscription_plan" || lower == "plan" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.SubscriptionPlan = strings.TrimSpace(s)
				}
			}
			if lower == "agent" || lower == "aiagent" || lower == "ai_agent" || lower == "agentname" || lower == "agent_name" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.Agent = strings.TrimSpace(s)
				}
			}
			if lower == "agentversion" || lower == "agent_version" || lower == "aiagentversion" || lower == "ai_agent_version" || lower == "copilotversion" || lower == "version" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.AgentVersion = strings.TrimSpace(s)
				}
			}
			if lower == "inputtokens" || lower == "input_tokens" || lower == "prompttokens" || lower == "prompt_tokens" || lower == "prompttokencount" || lower == "cachecreationinputtokens" || lower == "cache_creation_input_tokens" || lower == "cachereadinputtokens" || lower == "cache_read_input_tokens" || lower == "cacheread" || lower == "cachewrite" {
				summary.InputTokens += jsonInt(raw)
			}
			if lower == "tokensin" {
				summary.InputTokens += jsonInt(raw)
			}
			if lower == "outputtokens" || lower == "output_tokens" || lower == "completiontokens" || lower == "completion_tokens" || lower == "candidatestokencount" || lower == "reasoningoutputtokens" || lower == "reasoning_output_tokens" {
				summary.OutputTokens += jsonInt(raw)
			}
			if lower == "tokensout" {
				summary.OutputTokens += jsonInt(raw)
			}
			if lower == "input" && jsonInt(raw) > 0 {
				summary.InputTokens += jsonInt(raw)
			}
			if lower == "output" && jsonInt(raw) > 0 {
				summary.OutputTokens += jsonInt(raw)
			}
			if lower == "prompt_length" {
				summary.PromptLength += jsonInt(raw)
			}
			if isAIPathKey(lower) {
				switch value := raw.(type) {
				case string:
					paths := aiPathsFromValue(value, summary.CWD)
					for _, path := range paths {
						summary.Files[path] = true
					}
				case []any:
					for _, item := range value {
						s, ok := item.(string)
						if !ok {
							continue
						}
						for _, path := range aiPathsFromValue(s, summary.CWD) {
							summary.Files[path] = true
						}
					}
				}
			}
			if lower == "request" {
				if s, ok := raw.(string); ok {
					updateAIRooRequest(summary, s)
				}
			}
			if lower == "patchtext" || lower == "patch_text" {
				if s, ok := raw.(string); ok {
					updateAIPatchText(summary, s)
				}
			}
			if lower == "text" || lower == "input" || lower == "message" || lower == "prompt" || lower == "content" {
				if s, ok := raw.(string); ok {
					if parsed, ok := parseAIEmbeddedJSON(s); ok {
						updateAISummary(summary, parsed)
						continue
					}
					if summary.PromptLength == 0 {
						summary.PromptLength = len([]rune(aiPromptText(summary, s)))
					}
				}
			}
			if lower == "content" {
				if s, ok := raw.(string); ok {
					for _, path := range aiContentPaths(s, summary.CWD) {
						summary.Files[path] = true
					}
				}
			}
			updateAISummary(summary, raw)
		}
		updateAICodexPatch(summary, v)
		updateAIToolLineChanges(summary, v)
	case []any:
		for _, item := range v {
			updateAISummary(summary, item)
		}
	}
}

func aiPromptText(summary *aiTranscriptSummary, raw string) string {
	prompt := strings.TrimSpace(raw)
	if strings.EqualFold(summary.Source, "Codex") {
		return cleanCodexPrompt(prompt)
	}
	if strings.EqualFold(summary.Source, "Claude") {
		return cleanClaudePrompt(prompt)
	}
	return prompt
}

func cleanCodexPrompt(prompt string) string {
	prompt = strings.ReplaceAll(prompt, "\r\n", "\n")
	prompt = strings.TrimSpace(prompt)
	for _, marker := range []string{
		"## My request for Codex:",
		"## My request for Codex",
		"My request for Codex:",
		"User request:",
	} {
		if idx := strings.LastIndex(prompt, marker); idx >= 0 {
			return strings.TrimSpace(prompt[idx+len(marker):])
		}
	}
	if idx := strings.LastIndex(prompt, "</environment_context>"); idx >= 0 {
		return strings.TrimSpace(prompt[idx+len("</environment_context>"):])
	}
	return prompt
}

func cleanClaudePrompt(prompt string) string {
	prompt = strings.ReplaceAll(prompt, "\r\n", "\n")
	prompt = claudeSystemReminderRe.ReplaceAllString(prompt, "")
	return strings.TrimSpace(prompt)
}

func updateAIPatchText(summary *aiTranscriptSummary, patch string) {
	for path, lineChanges := range aiPatchLineChanges(patch, summary.CWD) {
		summary.Files[path] = true
		summary.FileWrites[path] = true
		summary.LineChanges[path] = lineChanges
	}
}

func updateAICodexPatch(summary *aiTranscriptSummary, payload map[string]any) {
	if !strings.EqualFold(summary.Source, "Codex") {
		return
	}
	payloadType := firstString(payload["type"])
	callID := firstString(payload["call_id"], payload["callId"])
	if payloadType == "custom_tool_call" && callID != "" {
		patch := codexPatchFromToolCall(firstString(payload["name"]), firstString(payload["input"]))
		if patch == "" {
			return
		}
		if summary.PendingPatch == nil {
			summary.PendingPatch = map[string]string{}
		}
		summary.PendingPatch[callID] = patch
		return
	}
	if !codexPatchCompletion(payloadType) || callID == "" {
		return
	}
	patch := ""
	if summary.PendingPatch != nil {
		patch = summary.PendingPatch[callID]
		delete(summary.PendingPatch, callID)
	}
	if patch == "" || !codexPatchSucceeded(payload) {
		return
	}
	updateAIPatchText(summary, patch)
}

func codexPatchFromToolCall(name, input string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "apply_patch":
		return input
	case "exec", "exec_command", "shell", "shell_command":
		return codexPatchFromShellInput(input)
	default:
		return ""
	}
}

func codexPatchFromShellInput(input string) string {
	command := input
	if parsed, ok := parseAIEmbeddedJSON(input); ok {
		if object, ok := parsed.(map[string]any); ok {
			command = firstString(object["cmd"], object["command"], object["script"])
		}
	}
	if !strings.Contains(command, "apply_patch") {
		return ""
	}
	start := strings.Index(command, "*** Begin Patch")
	end := strings.LastIndex(command, "*** End Patch")
	if start < 0 || end < start {
		return ""
	}
	end += len("*** End Patch")
	return strings.TrimSpace(command[start:end])
}

func codexPatchCompletion(payloadType string) bool {
	switch payloadType {
	case "patch_apply_end", "exec_command_end", "exec_end", "shell_command_end", "command_end":
		return true
	default:
		return false
	}
}

func codexPatchSucceeded(payload map[string]any) bool {
	if _, ok := payload["success"]; ok {
		return jsonBool(payload["success"])
	}
	for _, key := range []string{"exit_code", "exitCode", "code"} {
		if raw, ok := payload[key]; ok {
			return jsonInt(raw) == 0
		}
	}
	status := strings.ToLower(firstString(payload["status"], payload["result"]))
	return status == "" || status == "success" || status == "succeeded" || status == "completed" || status == "ok"
}

func aiPatchLineChanges(patch, cwd string) map[string]int {
	changes := map[string]int{}
	currentPath := ""
	for _, line := range strings.Split(patch, "\n") {
		if path, ok := aiPatchFilePath(line, cwd); ok {
			currentPath = path
			if _, exists := changes[currentPath]; !exists {
				changes[currentPath] = 0
			}
			continue
		}
		if currentPath == "" || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			changes[currentPath]++
		case strings.HasPrefix(line, "-"):
			changes[currentPath]--
		}
	}
	return changes
}

func aiPatchFilePath(line, cwd string) (string, bool) {
	for _, prefix := range []string{"*** Update File:", "*** Add File:"} {
		if strings.HasPrefix(line, prefix) {
			path := normalizeAIPath(strings.TrimSpace(strings.TrimPrefix(line, prefix)), cwd)
			return path, path != ""
		}
	}
	return "", false
}

func updateAICopilotCLIMetadata(summary *aiTranscriptSummary, payload map[string]any) {
	eventType := strings.ToLower(firstString(payload["type"]))
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return
	}
	if eventType == "session.start" {
		summary.Agent = "github-copilot-cli"
	}
	if eventType == "session.shutdown" {
		addAITokenDetails(summary, data)
	}
	telemetry, _ := data["toolTelemetry"].(map[string]any)
	if telemetry == nil {
		return
	}
	paths := aiToolTelemetryPaths(telemetry, summary.CWD)
	if len(paths) == 0 {
		return
	}
	lineChanges, hasLineChanges := aiToolTelemetryLineChanges(telemetry)
	for _, path := range paths {
		summary.Files[path] = true
		summary.FileWrites[path] = true
		if hasLineChanges {
			summary.LineChanges[path] = lineChanges
		}
	}
}

func addAITokenDetails(summary *aiTranscriptSummary, data map[string]any) {
	tokenDetails, _ := data["tokenDetails"].(map[string]any)
	if tokenDetails == nil {
		return
	}
	if input, ok := tokenDetails["input"].(map[string]any); ok {
		summary.InputTokens += jsonInt(input["tokenCount"])
	}
	if output, ok := tokenDetails["output"].(map[string]any); ok {
		summary.OutputTokens += jsonInt(output["tokenCount"])
	}
}

func aiToolTelemetryPaths(telemetry map[string]any, cwd string) []string {
	restricted, _ := telemetry["restrictedProperties"].(map[string]any)
	if restricted == nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"filePaths", "addedPaths", "deletedPaths"} {
		paths = append(paths, aiPathsFromAny(restricted[key], cwd)...)
	}
	return paths
}

func aiToolTelemetryLineChanges(telemetry map[string]any) (int, bool) {
	metrics, _ := telemetry["metrics"].(map[string]any)
	if metrics == nil {
		return 0, false
	}
	added := jsonInt(metrics["linesAdded"])
	removed := jsonInt(metrics["linesRemoved"])
	if added == 0 && removed == 0 {
		return 0, false
	}
	return added - removed, true
}

func aiPathsFromAny(value any, cwd string) []string {
	switch v := value.(type) {
	case string:
		return aiPathsFromValue(v, cwd)
	case []any:
		var paths []string
		for _, item := range v {
			paths = append(paths, aiPathsFromAny(item, cwd)...)
		}
		return paths
	default:
		return nil
	}
}

func aiPathsFromValue(value, cwd string) []string {
	if path := normalizeAIPath(value, cwd); path != "" {
		return []string{path}
	}
	if parsed, ok := parseAIEmbeddedJSON(value); ok {
		return aiPathsFromAny(parsed, cwd)
	}
	return nil
}

func parseAIEmbeddedJSON(value string) (any, bool) {
	value = strings.TrimSpace(value)
	if value == "" || (value[0] != '{' && value[0] != '[') {
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

func updateAIRooRequest(summary *aiTranscriptSummary, request string) {
	if cwd := rooRequestCWD(request); cwd != "" {
		summary.CWD = expandHome(cwd)
	}
	if task := rooRequestTask(request); task != "" && summary.PromptLength == 0 {
		summary.PromptLength = len(task)
	}
}

func rooRequestTask(request string) string {
	return betweenTrimmed(request, "<task>", "</task>")
}

func rooRequestCWD(request string) string {
	const prefix = "# Current Working Directory ("
	start := strings.Index(request, prefix)
	if start < 0 {
		return ""
	}
	rest := request[start+len(prefix):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func betweenTrimmed(value, start, end string) string {
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		return ""
	}
	rest := value[startIndex+len(start):]
	endIndex := strings.Index(rest, end)
	if endIndex < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:endIndex])
}

func updateAIToolLineChanges(summary *aiTranscriptSummary, payload map[string]any) {
	if functionCall, ok := payload["functionCall"].(map[string]any); ok {
		updateAIFunctionCall(summary, functionCall)
	}
	if firstString(payload["name"], payload["toolName"], payload["tool_call_name"]) != "" {
		updateAIFunctionCall(summary, payload)
	}
	if parameters, ok := payload["args"].(map[string]any); ok {
		if patchText := firstString(parameters["patchText"], parameters["patch_text"]); patchText != "" {
			cwd := firstString(parameters["workdir"], parameters["cwd"])
			if cwd == "" {
				cwd = summary.CWD
			}
			for path, lineChanges := range aiPatchLineChanges(patchText, cwd) {
				summary.Files[path] = true
				summary.FileWrites[path] = true
				summary.LineChanges[path] = lineChanges
			}
		}
	}
	path := firstString(payload["path"], payload["file_path"], payload["filePath"])
	if path == "" {
		if parameters, ok := payload["parameters"].(map[string]any); ok {
			path = firstString(parameters["path"], parameters["file_path"], parameters["filePath"])
		}
	}
	normalized := normalizeAIPath(path, summary.CWD)
	if normalized == "" {
		return
	}
	diff := firstString(payload["diff"])
	content := firstString(payload["content"])
	tool := strings.ToLower(firstString(payload["tool"], payload["toolCallName"], payload["tool_call_name"]))
	switch {
	case diff != "":
		summary.Files[normalized] = true
		summary.FileWrites[normalized] = true
		summary.LineChanges[normalized] = aiReplacementDiffLineChanges(diff)
	case content != "" && (strings.Contains(tool, "write") || strings.Contains(tool, "newfile") || strings.Contains(tool, "new_file") || strings.Contains(tool, "created")):
		summary.Files[normalized] = true
		summary.FileWrites[normalized] = true
		summary.LineChanges[normalized] = countAIContentLines(content)
	case strings.Contains(tool, "read"):
		summary.Files[normalized] = true
		summary.FileWrites[normalized] = false
	}
}

func updateAIFunctionCall(summary *aiTranscriptSummary, payload map[string]any) {
	tool := strings.ToLower(firstString(payload["name"], payload["toolName"], payload["tool_call_name"]))
	args, ok := payload["args"].(map[string]any)
	if !ok {
		if raw := firstString(payload["args"]); raw != "" {
			if parsed, parsedOK := parseAIEmbeddedJSON(raw); parsedOK {
				args, _ = parsed.(map[string]any)
			}
		}
	}
	if args == nil {
		return
	}
	path := normalizeAIPath(firstString(args["file_path"], args["filePath"], args["path"]), summary.CWD)
	if path == "" {
		return
	}
	summary.Files[path] = true
	switch {
	case strings.Contains(tool, "read"):
		summary.FileWrites[path] = false
		summary.LineChanges[path] = 0
	case strings.Contains(tool, "write") || strings.Contains(tool, "create"):
		summary.FileWrites[path] = true
		summary.LineChanges[path] = firstAIResultLineChanges(payload, countAIContentLines(firstString(args["content"])))
	case strings.Contains(tool, "edit") || strings.Contains(tool, "replace"):
		summary.FileWrites[path] = true
		fallback := countAIContentLines(firstString(args["new_string"], args["newString"])) - countAIContentLines(firstString(args["old_string"], args["oldString"]))
		summary.LineChanges[path] = firstAIResultLineChanges(payload, fallback)
	}
}

func firstAIResultLineChanges(payload map[string]any, fallback int) int {
	display, _ := payload["resultDisplay"].(map[string]any)
	if display == nil {
		return fallback
	}
	diffStat, _ := display["diffStat"].(map[string]any)
	if diffStat == nil {
		return fallback
	}
	added := jsonInt(firstAny(diffStat["model_added_lines"], diffStat["modelAddedLines"], diffStat["linesAdded"]))
	removed := jsonInt(firstAny(diffStat["model_removed_lines"], diffStat["modelRemovedLines"], diffStat["linesRemoved"]))
	return added - removed
}

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func aiReplacementDiffLineChanges(diff string) int {
	lines := strings.Split(diff, "\n")
	oldLines := 0
	newLines := 0
	total := 0
	state := ""
	flush := func() {
		if state == "" {
			return
		}
		total += newLines - oldLines
		oldLines = 0
		newLines = 0
		state = ""
	}
	for _, line := range lines {
		switch line {
		case "<<<<<<< SEARCH":
			flush()
			state = "old"
		case "=======":
			if state == "old" {
				state = "new"
			}
		case ">>>>>>> REPLACE":
			flush()
		default:
			switch state {
			case "old":
				oldLines++
			case "new":
				newLines++
			}
		}
	}
	flush()
	return total
}

func countAIContentLines(content string) int {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func aiSummaryHeartbeats(summary aiTranscriptSummary, opts Options) []Heartbeat {
	sessionID := first(summary.SessionID, "unknown")
	project, branch := aiProject(summary.CWD, opts)
	baseTime := float64(summary.LastActivity.UnixNano()) / 1e9
	app := Heartbeat{
		AIAgent:            first(summary.Agent, strings.ToLower(summary.Source)),
		AIAgentVersion:     summary.AgentVersion,
		AISession:          sessionID,
		AIInputTokens:      intPointer(summary.InputTokens),
		AIModel:            summary.Model,
		AIOutputTokens:     intPointer(summary.OutputTokens),
		AIProvider:         summary.Provider,
		AIPromptLength:     intPointer(summary.PromptLength),
		AISubscriptionPlan: summary.SubscriptionPlan,
		Branch:             branch,
		Category:           aiCodingCategory,
		Entity:             summary.Source + " " + sessionID,
		EntityType:         "app",
		MachineName:        machineName(opts.Hostname),
		Plugin:             first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript"),
		Project:            first(opts.Project, project, opts.AlternateProject),
		Time:               baseTime,
		UserAgent:          userAgent(first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript")),
	}
	heartbeats := []Heartbeat{app}
	if len(summary.FileEvents) > 0 {
		for i, event := range summary.FileEvents {
			if i >= 20 || !fileExists(event.Path) {
				continue
			}
			fileProject, fileBranch, _ := detectProject(event.Path, opts)
			heartbeats = append(heartbeats, Heartbeat{
				AIAgent:            first(summary.Agent, strings.ToLower(summary.Source)),
				AIAgentVersion:     summary.AgentVersion,
				AILineChanges:      event.LineChanges,
				AISession:          sessionID,
				AIModel:            summary.Model,
				AIProvider:         summary.Provider,
				AISubscriptionPlan: summary.SubscriptionPlan,
				Branch:             first(opts.Branch, fileBranch, branch, opts.AlternateBranch),
				Category:           aiCodingCategory,
				Entity:             event.Path,
				EntityType:         "file",
				IsWrite:            event.IsWrite,
				Language:           detectLanguage(event.Path),
				MachineName:        machineName(opts.Hostname),
				Plugin:             first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript"),
				Project:            first(opts.Project, fileProject, project, opts.AlternateProject),
				Time:               baseTime + float64(i+1)/1000,
				UserAgent:          userAgent(first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript")),
			})
		}
		return heartbeats
	}
	files := sortedAIPaths(summary.Files)
	for i, path := range files {
		if i >= 20 {
			break
		}
		if !fileExists(path) {
			continue
		}
		fileProject, fileBranch, _ := detectProject(path, opts)
		var aiLineChanges *int
		if lineChanges, ok := summary.LineChanges[path]; ok {
			aiLineChanges = intPointerAllowZero(lineChanges)
		}
		isWrite, ok := summary.FileWrites[path]
		if !ok {
			isWrite = true
		}
		heartbeats = append(heartbeats, Heartbeat{
			AIAgent:            first(summary.Agent, strings.ToLower(summary.Source)),
			AIAgentVersion:     summary.AgentVersion,
			AILineChanges:      aiLineChanges,
			AISession:          sessionID,
			AIModel:            summary.Model,
			AIProvider:         summary.Provider,
			AISubscriptionPlan: summary.SubscriptionPlan,
			Branch:             first(opts.Branch, fileBranch, branch, opts.AlternateBranch),
			Category:           aiCodingCategory,
			Entity:             path,
			EntityType:         "file",
			IsWrite:            isWrite,
			Language:           detectLanguage(path),
			MachineName:        machineName(opts.Hostname),
			Plugin:             first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript"),
			Project:            first(opts.Project, fileProject, project, opts.AlternateProject),
			Time:               baseTime + float64(i+1)/1000,
			UserAgent:          userAgent(first(opts.Plugin, strings.ToLower(summary.Source)+"/transcript")),
		})
	}
	return heartbeats
}

func intPointerAllowZero(value int) *int {
	return &value
}

func aiTranscriptSessionID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base != "transcript" {
		return base
	}
	dir := filepath.Dir(path)
	if filepath.Base(dir) != "logs" {
		return base
	}
	sessionDir := filepath.Dir(filepath.Dir(dir))
	sessionID := filepath.Base(sessionDir)
	if sessionID == "." || sessionID == string(filepath.Separator) || strings.TrimSpace(sessionID) == "" {
		return base
	}
	return sessionID
}

func aiContentPaths(content, cwd string) []string {
	matches := antigravityCodeActionPathRe.FindAllStringSubmatch(content, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := normalizeAIPath(match[1], cwd)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func aiProject(cwd string, opts Options) (string, string) {
	if cwd == "" {
		return "", ""
	}
	root := findProjectRoot(cwd)
	if root == "" {
		root = cwd
	}
	project, branch, _ := detectProject(filepath.Join(root, ".ai-session"), Options{
		AlternateBranch:  opts.AlternateBranch,
		AlternateProject: opts.AlternateProject,
		Branch:           opts.Branch,
		Config:           opts.Config,
		EntityType:       "app",
		Project:          opts.Project,
		ProjectFolder:    root,
	})
	return project, branch
}

func isAIPathKey(key string) bool {
	switch key {
	case "path", "paths", "fspath", "fs_path", "uri", "uris", "filepath", "filepaths", "file_path", "file_paths", "filename", "filenames", "file", "files", "originalfile", "original_file", "addedpaths", "added_paths", "deletedpaths", "deleted_paths":
		return true
	default:
		return false
	}
}

var aiPathRe = regexp.MustCompile(`^[A-Za-z0-9_./~:\\ -]+\.[A-Za-z0-9]{1,12}$`)

func normalizeAIPath(value, cwd string) string {
	value = strings.TrimSpace(value)
	value = normalizeAIFileURI(value)
	if value == "" || strings.Contains(value, "\n") || !aiPathRe.MatchString(value) {
		return ""
	}
	value = expandHome(value)
	if !filepath.IsAbs(value) && cwd != "" {
		value = filepath.Join(cwd, value)
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return filepath.Clean(value)
}

func normalizeAIFileURI(value string) string {
	if !strings.HasPrefix(strings.ToLower(value), "file://") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimPrefix(value, "file://")
	}
	path := parsed.Path
	if path == "" {
		path = strings.TrimPrefix(value, "file://")
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		path = string(filepath.Separator) + filepath.Join(parsed.Host, path)
	}
	return path
}

func sortedAIPaths(paths map[string]bool) []string {
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
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
