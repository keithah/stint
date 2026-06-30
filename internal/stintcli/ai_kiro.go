package stintcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
	seenFiles := 0
	err := filepath.WalkDir(sessionRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
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
		data, err := readAITranscriptFile(path)
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

func kiroApplyExecutions(root string, sessions map[string]kiroSessionInfo, summaries map[string]*aiTranscriptSummary, after time.Time) error {
	seenFiles := 0
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.Contains(path, string(filepath.Separator)+"workspace-sessions"+string(filepath.Separator)) {
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
		data, err := readAITranscriptFile(path)
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
