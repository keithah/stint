package stintcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	seenFiles := 0
	err := filepath.WalkDir(source.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !source.matches(entry.Name()) {
			return nil
		}
		if seenFiles >= maxAITranscriptFiles {
			return filepath.SkipAll
		}
		seenFiles++
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(after) {
			return nil
		}
		if info.Size() > maxAITranscriptFileBytes {
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
