package stintcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var errHeartbeatFiltered = errors.New("heartbeat filtered")

var (
	vimModelineRegex       = regexp.MustCompile(`(?m)(?:vi|vim|ex)(?:[<=>]?\d*)?:.*(?:ft|filetype|syn|syntax)=([^:\s]+)`)
	forthFuncRegex         = regexp.MustCompile(`:[^\n\r]+;[\n\r]`)
	pathSeparatorRunsRegex = regexp.MustCompile(`[\\/]+`)
)

var wakaTimeExactLanguageFiles = map[string]string{
	".ruby-version": "Ruby",
	"crontab":       "Crontab",
}

var wakaTimeExtensionLanguages = map[string]string{
	".cfm":              "ColdFusion",
	".cfml":             "ColdFusion",
	".fhtml":            "Velocity",
	".gs":               "Gosu",
	".gsp":              "Gosu",
	".gsx":              "Gosu",
	".i":                "SWIG",
	".inc":              "Pawn",
	".j":                "Objective-J",
	".jade":             "Pug",
	".kif":              "newLisp",
	".lasso8":           "Lasso",
	".lasso9":           "Lasso",
	".lsp":              "newLisp",
	".marko":            "Marko",
	".mo":               "Modelica",
	".mustache":         "Mustache",
	".nl":               "newLisp",
	".pug":              "Pug",
	".pwn":              "Pawn",
	".sketch":           "Sketch Drawing",
	".slim":             "Slim",
	".swg":              "SWIG",
	".sublime-settings": "Sublime Text Config",
	".vark":             "Gosu",
	".vm":               "Velocity",
	".xaml":             "XAML",
	".xpl":              "XSLT",
}

const (
	wakaRegexMatchTimeout = 100 * time.Millisecond
	maxFileStatsBytes     = 5 * 1024 * 1024
)

// Heartbeat is the WakaTime-compatible heartbeat payload sent by the CLI.
type Heartbeat struct {
	AILineChanges      *int           `json:"ai_line_changes,omitempty"`
	AIAgent            string         `json:"ai_agent,omitempty"`
	AIAgentComplexity  string         `json:"ai_agent_complexity,omitempty"`
	AIAgentVersion     string         `json:"ai_agent_version,omitempty"`
	AIInputTokens      *int           `json:"ai_input_tokens,omitempty"`
	AIModel            string         `json:"ai_model,omitempty"`
	AIOutputTokens     *int           `json:"ai_output_tokens,omitempty"`
	AIProvider         string         `json:"ai_provider,omitempty"`
	AIPromptLength     *int           `json:"ai_prompt_length,omitempty"`
	AISession          string         `json:"ai_session,omitempty"`
	AISubscriptionPlan string         `json:"ai_subscription_plan,omitempty"`
	AlternateBranch    string         `json:"-"`
	AlternateLanguage  string         `json:"-"`
	AlternateProject   string         `json:"-"`
	Branch             string         `json:"branch,omitempty"`
	Category           string         `json:"category,omitempty"`
	CommitHash         string         `json:"commit_hash,omitempty"`
	CursorPosition     *int           `json:"cursorpos,omitempty"`
	Dependencies       []string       `json:"dependencies,omitempty"`
	Editor             string         `json:"editor,omitempty"`
	EditorVersion      string         `json:"editor_version,omitempty"`
	Entity             string         `json:"entity"`
	EntityType         string         `json:"type"`
	HumanLineChanges   *int           `json:"human_line_changes,omitempty"`
	IsUnsavedEntity    bool           `json:"-"`
	IsWrite            bool           `json:"is_write,omitempty"`
	IsWriteSet         bool           `json:"-"`
	Language           string         `json:"language,omitempty"`
	LineNumber         *int           `json:"lineno,omitempty"`
	Lines              *int           `json:"lines,omitempty"`
	LocalFile          string         `json:"-"`
	MachineName        string         `json:"machine_name,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	Plugin             string         `json:"plugin,omitempty"`
	PluginVersion      string         `json:"plugin_version,omitempty"`
	Project            string         `json:"project,omitempty"`
	ProjectRootCount   *int           `json:"project_root_count,omitempty"`
	Time               float64        `json:"time"`
	UserAgent          string         `json:"user_agent,omitempty"`
}

func (h Heartbeat) MarshalJSON() ([]byte, error) {
	type Alias Heartbeat
	data, err := marshalJSONNoHTMLEscape(Alias(h))
	if err != nil {
		return nil, err
	}
	if !h.IsWriteSet || h.IsWrite {
		return data, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	payload["is_write"] = false
	return marshalJSONNoHTMLEscape(payload)
}

func marshalJSONNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (h Heartbeat) ID() string {
	cursorPos := "nil"
	if h.CursorPosition != nil {
		cursorPos = fmt.Sprint(*h.CursorPosition)
	}
	category := "undefined"
	if h.Category != "" {
		category = h.Category
	}
	project := "unset"
	if h.Project != "" {
		project = h.Project
	}
	branch := "unset"
	if h.Branch != "" {
		branch = h.Branch
	}
	return fmt.Sprintf("%f-%s-%s-%s-%s-%s-%s-%t",
		h.Time,
		cursorPos,
		h.EntityType,
		category,
		project,
		branch,
		h.Entity,
		h.IsWrite,
	)
}

func BuildHeartbeat(o Options) (Heartbeat, error) {
	entity := first(o.Entity, o.LocalFile)
	if entity == "" {
		return Heartbeat{}, fmt.Errorf("--entity is required")
	}
	entity, localFile, statsFile, cleanupRemoteStatsFile, remoteEntity, err := resolveStatsFile(entity, o.EntityType, o.LocalFile, o.IsUnsavedEntity)
	if cleanupRemoteStatsFile != nil {
		defer cleanupRemoteStatsFile()
	}
	if err != nil {
		return Heartbeat{}, err
	}
	o.LocalFile = localFile
	if o.EntityType == "file" && !o.IsUnsavedEntity {
		if _, err := os.Stat(statsFile); err != nil {
			return Heartbeat{}, fmt.Errorf("%w: entity file does not exist: %s", errHeartbeatFiltered, statsFile)
		}
	}
	project, branch, root := detectProject(statsFile, o)
	if o.IncludeOnlyProjectFile && !remoteEntity && (root == "" || !fileExists(filepath.Join(root, ".wakatime-project"))) {
		return Heartbeat{}, fmt.Errorf("%w: entity excluded because project has no .wakatime-project", errHeartbeatFiltered)
	}
	if o.ExcludeUnknownProject && project == "" {
		return Heartbeat{}, fmt.Errorf("%w: project could not be detected", errHeartbeatFiltered)
	}
	if skip, err := excluded(entity, o.Include, o.Exclude); err != nil || skip {
		if err != nil {
			return Heartbeat{}, err
		}
		return Heartbeat{}, fmt.Errorf("%w: entity excluded by filters", errHeartbeatFiltered)
	}
	rootCount := projectRootCount(root)
	resolvedLanguage := first(o.Language, detectLanguageWithGuess(statsFile, o.GuessLanguage), detectLanguageWithGuess(entity, o.GuessLanguage), o.AlternateLanguage)
	dependencies := []string(nil)
	if o.EntityType == "file" && !o.IsUnsavedEntity {
		dependencies = detectDependenciesForLanguage(statsFile, resolvedLanguage)
	}
	hb := Heartbeat{
		AIAgent:            o.AIAgent,
		AIAgentComplexity:  o.AIAgentComplexity,
		AIAgentVersion:     o.AIAgentVersion,
		AIModel:            o.AIModel,
		AIProvider:         o.AIProvider,
		AISession:          o.AISession,
		AISubscriptionPlan: o.AISubscriptionPlan,
		Branch:             first(o.Branch, branch, o.AlternateBranch),
		Category:           categoryForHeartbeat(entity, o),
		CommitHash:         o.CommitHash,
		Dependencies:       dependencies,
		Editor:             o.Editor,
		EditorVersion:      o.EditorVersion,
		Entity:             entity,
		EntityType:         o.EntityType,
		IsUnsavedEntity:    o.IsUnsavedEntity,
		IsWrite:            o.Write,
		IsWriteSet:         o.WriteSet,
		Language:           resolvedLanguage,
		MachineName:        machineName(o.Hostname),
		Metadata:           parseMetadata(o.Metadata),
		Plugin:             o.Plugin,
		PluginVersion:      o.PluginVersion,
		Project:            first(o.Project, project, o.AlternateProject),
		ProjectRootCount:   rootCount,
		Time:               o.Time,
		UserAgent:          userAgent(o.Plugin),
	}
	if hb.Time == 0 {
		hb.Time = float64(time.Now().UnixNano()) / 1e9
	}
	assignInt := func(v int, set bool) *int {
		if v == 0 && !set {
			return nil
		}
		return &v
	}
	hb.AILineChanges = assignInt(o.AILineChanges, o.AILineChangesSet)
	hb.AIInputTokens = assignInt(o.AIInputTokens, o.AIInputTokensSet)
	hb.AIOutputTokens = assignInt(o.AIOutputTokens, o.AIOutputTokensSet)
	hb.AIPromptLength = assignInt(o.AIPromptLength, o.AIPromptLengthSet)
	hb.HumanLineChanges = assignInt(o.HumanLineChanges, o.HumanLineChangesSet)
	hb.CursorPosition = assignInt(o.CursorPosition, o.CursorPositionSet)
	hb.LineNumber = assignInt(o.LineNumber, o.LineNumberSet)
	lines := o.LinesInFile
	if !o.LinesInFileSet && !o.IsUnsavedEntity {
		lines = countLines(statsFile)
	}
	hb.Lines = assignInt(lines, o.LinesInFileSet)
	hb = sanitizeHeartbeat(hb, heartbeatSanitizeInput{
		hideBranchNames:   o.HideBranchNames,
		hideDependencies:  o.HideDependencies,
		hideFileNames:     o.HideFileNames,
		hideProjectFolder: o.HideProjectFolder,
		hideProjectNames:  o.HideProjectNames,
		projectRoot:       root,
		remoteEntity:      remoteEntity,
	})
	return hb, nil
}

func modifyLocalFileEntity(entity string) string {
	info, err := os.Stat(entity)
	if err != nil || !info.IsDir() {
		return entity
	}
	lower := strings.ToLower(entity)
	switch {
	case strings.HasSuffix(lower, ".playground"), strings.HasSuffix(lower, ".xcplayground"), strings.HasSuffix(lower, ".xcplaygroundpage"):
		return filepath.Join(entity, "Contents.swift")
	case strings.HasSuffix(lower, ".xcodeproj"):
		return filepath.Join(entity, "project.pbxproj")
	default:
		return entity
	}
}

func (p wakaPattern) MatchString(s string) bool {
	if p.re != nil {
		return p.re.MatchString(s)
	}
	if p.re2 != nil {
		matched, err := p.re2.MatchString(s)
		return err == nil && matched
	}
	return false
}

func (p wakaPattern) FindStringSubmatch(s string) []string {
	if p.re != nil {
		return p.re.FindStringSubmatch(s)
	}
	if p.re2 == nil {
		return nil
	}
	match, err := p.re2.FindStringMatch(s)
	if err != nil || match == nil {
		return nil
	}
	groups := match.Groups()
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		if len(group.Captures) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, group.Captures[0].String())
	}
	return out
}
