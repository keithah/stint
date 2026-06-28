package services

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"
)

type HeartbeatDefaults struct {
	Plugin          string
	PluginVersion   string
	Editor          string
	EditorVersion   string
	OperatingSystem string
	Architecture    string
	AIAgent         string
	AIAgentVersion  string
}

func PrepareHeartbeat(heartbeat *Heartbeat, defaults HeartbeatDefaults) {
	if heartbeat.Type == "" {
		heartbeat.Type = "file"
	}
	if heartbeat.Category == "" {
		heartbeat.Category = "coding"
	}
	if heartbeat.CommitHash == "" {
		heartbeat.CommitHash = heartbeat.Revision
	}
	if heartbeat.Revision == "" {
		heartbeat.Revision = heartbeat.CommitHash
	}
	if heartbeat.Editor == "" {
		heartbeat.Editor = defaults.Editor
	}
	if heartbeat.EditorVersion == "" {
		heartbeat.EditorVersion = defaults.EditorVersion
	}
	if heartbeat.OperatingSystem == "" {
		heartbeat.OperatingSystem = defaults.OperatingSystem
	}
	if heartbeat.Plugin == "" {
		heartbeat.Plugin = defaults.Plugin
	}
	if heartbeat.PluginVersion == "" {
		heartbeat.PluginVersion = defaults.PluginVersion
	}
	if heartbeat.Architecture == "" {
		heartbeat.Architecture = defaults.Architecture
	}
	if heartbeat.AIAgent == "" {
		heartbeat.AIAgent = defaults.AIAgent
	}
	if heartbeat.AIAgentVersion == "" {
		heartbeat.AIAgentVersion = defaults.AIAgentVersion
	}
	if heartbeat.AIAgent == "" && strings.EqualFold(heartbeat.Editor, "codex") && heartbeatHasAIFields(*heartbeat) {
		heartbeat.AIAgent = "gpt"
	}
	if heartbeat.AIProvider == "" {
		heartbeat.AIProvider = inferAIProvider(*heartbeat)
	}
	if heartbeat.MachineName == "" {
		heartbeat.MachineName = derivedMachineName(*heartbeat)
	}
	applyProjectDetection(heartbeat)
	if heartbeat.Language == "" {
		heartbeat.Language = inferLanguageFromEntity(heartbeat.Entity)
	}
}

func derivedMachineName(heartbeat Heartbeat) string {
	editor := strings.TrimSpace(heartbeat.Editor)
	osName := strings.TrimSpace(heartbeat.OperatingSystem)
	if editor == "" || editor == "Unknown" || osName == "" || osName == "Unknown" {
		return ""
	}
	return strings.ToLower(editor + "-" + osName)
}

func heartbeatHasAIFields(heartbeat Heartbeat) bool {
	return heartbeat.AILineChanges != nil || heartbeat.HumanLineChanges != nil || heartbeat.AIInputTokens != nil ||
		heartbeat.AIOutputTokens != nil || heartbeat.AIPromptLength != nil || heartbeat.AISession != ""
}

func inferAIProvider(heartbeat Heartbeat) string {
	text := strings.ToLower(strings.Join([]string{heartbeat.AIAgent, heartbeat.AIModel, heartbeat.Editor, heartbeat.Plugin}, " "))
	switch {
	case strings.Contains(text, "claude") || strings.Contains(text, "anthropic"):
		return "anthropic"
	case strings.Contains(text, "gpt") || strings.Contains(text, "openai") || strings.Contains(text, "codex"):
		return "openai"
	case strings.Contains(text, "gemini") || strings.Contains(text, "google"):
		return "google"
	case strings.Contains(text, "copilot"):
		return "github"
	default:
		return ""
	}
}

func inferLanguageFromEntity(entity string) string {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return ""
	}
	filename := filepath.Base(entity)
	lowerFilename := strings.ToLower(filename)
	lowerEntity := strings.ToLower(entity)
	if lowerFilename == "go.mod" {
		return "Go"
	}
	if lowerFilename == "cmakelists.txt" {
		return "CMake"
	}
	if lowerFilename == "dockerfile" || strings.HasSuffix(lowerFilename, ".dockerfile") {
		return "Docker"
	}
	if lowerFilename == "makefile" {
		return "Makefile"
	}
	if strings.HasSuffix(lowerEntity, ".pbxproj") {
		return "Xcode Project"
	}
	if lexer := lexers.Match(filename); lexer != nil {
		return normalizeChromaLanguage(lexer.Config().Name)
	}
	extensions := map[string]string{
		".bash":  "Bash",
		".c":     "C",
		".cc":    "C++",
		".cpp":   "C++",
		".css":   "CSS",
		".go":    "Go",
		".h":     "C",
		".hpp":   "C++",
		".html":  "HTML",
		".java":  "Java",
		".js":    "JavaScript",
		".json":  "JSON",
		".jsx":   "JavaScript",
		".kt":    "Kotlin",
		".kts":   "Kotlin",
		".lua":   "Lua",
		".m":     "Objective-C",
		".md":    "Markdown",
		".mjs":   "JavaScript",
		".mm":    "Objective-C++",
		".py":    "Python",
		".rb":    "Ruby",
		".rs":    "Rust",
		".sh":    "Bash",
		".sql":   "SQL",
		".swift": "Swift",
		".toml":  "TOML",
		".ts":    "TypeScript",
		".tsx":   "TypeScript",
		".yaml":  "YAML",
		".yml":   "YAML",
		".zsh":   "Zsh",
	}
	for extension, language := range extensions {
		if strings.HasSuffix(lowerEntity, extension) {
			return language
		}
	}
	return ""
}

func normalizeChromaLanguage(language string) string {
	switch language {
	case "Go":
		return "Go"
	case "GAS":
		return "Assembly"
	case "HTML":
		return "HTML"
	case "JSON":
		return "JSON"
	case "TypeScript":
		return "TypeScript"
	case "vue":
		return "Vue"
	default:
		return language
	}
}

func applyProjectDetection(heartbeat *Heartbeat) {
	if heartbeat.Entity == "" {
		return
	}
	start := heartbeat.Entity
	if !isDirectory(start) {
		start = filepath.Dir(start)
	}
	if result, ok := detectWakaTimeProjectFile(start); ok {
		if heartbeat.Project == "" {
			heartbeat.Project = result.Project
		}
		if heartbeat.Branch == "" {
			heartbeat.Branch = result.Branch
		}
	}
	if heartbeat.Project != "" && heartbeat.Branch != "" {
		return
	}
	if result, ok := detectGitProject(start); ok {
		if heartbeat.Project == "" {
			heartbeat.Project = result.Project
		}
		if heartbeat.Branch == "" {
			heartbeat.Branch = result.Branch
		}
	}
}

type projectDetectionResult struct {
	Project string
	Branch  string
}

func detectWakaTimeProjectFile(start string) (projectDetectionResult, bool) {
	for _, dir := range parentDirs(start) {
		path := filepath.Join(dir, ".wakatime-project")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		project := filepath.Base(dir)
		if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
			project = strings.TrimSpace(lines[0])
		}
		branch := ""
		if len(lines) > 1 {
			branch = strings.TrimSpace(lines[1])
		}
		return projectDetectionResult{Project: project, Branch: branch}, true
	}
	return projectDetectionResult{}, false
}

func detectGitProject(start string) (projectDetectionResult, bool) {
	for _, dir := range parentDirs(start) {
		gitPath := filepath.Join(dir, ".git")
		if isDirectory(gitPath) {
			return projectDetectionResult{Project: filepath.Base(dir), Branch: readGitBranch(filepath.Join(gitPath, "HEAD"))}, true
		}
		data, err := os.ReadFile(gitPath)
		if err != nil || !strings.HasPrefix(string(data), "gitdir:") {
			continue
		}
		gitDir := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir:"))
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Clean(filepath.Join(dir, gitDir))
		}
		return projectDetectionResult{Project: filepath.Base(dir), Branch: readGitBranch(filepath.Join(gitDir, "HEAD"))}, true
	}
	return projectDetectionResult{}, false
}

func readGitBranch(headPath string) string {
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(head, prefix) {
		return strings.TrimPrefix(head, prefix)
	}
	return ""
}

func parentDirs(start string) []string {
	start = filepath.Clean(start)
	dirs := []string{}
	for i := 0; i < 500 && start != "." && start != string(filepath.Separator); i++ {
		dirs = append(dirs, start)
		parent := filepath.Dir(start)
		if parent == start {
			break
		}
		start = parent
	}
	return dirs
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ValidateHeartbeat(heartbeat Heartbeat) error {
	return ValidateHeartbeatAt(heartbeat, time.Now())
}

func ValidateHeartbeatAt(heartbeat Heartbeat, now time.Time) error {
	if strings.TrimSpace(heartbeat.Entity) == "" {
		return errors.New("entity is required")
	}
	if heartbeat.Time == 0 || math.IsNaN(heartbeat.Time) || math.IsInf(heartbeat.Time, 0) {
		return errors.New("time is required")
	}
	t := time.Unix(int64(heartbeat.Time), 0)
	if t.Before(now.AddDate(-1, 0, 0)) || t.After(now.Add(24*time.Hour)) {
		return errors.New("time must be within the last year and not more than 24 hours in the future")
	}
	return nil
}
