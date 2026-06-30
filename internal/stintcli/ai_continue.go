package stintcli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
	seenFiles := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".jsonl" {
			return nil
		}
		if seenFiles >= maxAITranscriptFiles {
			return filepath.SkipAll
		}
		seenFiles++
		info, err := entry.Info()
		if err != nil || info.Size() > maxAITranscriptFileBytes {
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
	data, err := readAITranscriptFile(path)
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
