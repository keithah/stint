package stintcli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/dlclark/regexp2"
	"github.com/google/uuid"
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

var remoteEntityRegex = regexp.MustCompile(`(?i)^(ssh|sftp)://`)

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

func resolveStatsFile(entity, entityType, localFile string, isUnsaved bool) (resolvedEntity, resolvedLocalFile, statsFile string, cleanup func(), remoteEntity bool, err error) {
	remoteEntity = entityType == "file" && !isUnsaved && isRemoteEntity(entity)
	if !remoteEntity {
		expanded, err := expandHomeStrict(entity)
		if err != nil {
			return "", "", "", nil, false, fmt.Errorf("failed expanding entity: %w", err)
		}
		entity = expanded
	}
	statsFile = entity
	if entityType != "file" {
		return entity, localFile, statsFile, nil, remoteEntity, nil
	}
	if localFile != "" {
		expanded, err := expandHomeStrict(localFile)
		if err != nil {
			return "", "", "", nil, remoteEntity, fmt.Errorf("failed expanding local-file: %w", err)
		}
		localFile = expanded
		statsFile = normalizeLocalEntityPath(localFile)
		statsFile = modifyLocalFileEntity(statsFile)
		return entity, localFile, statsFile, nil, remoteEntity, nil
	}
	if remoteEntity {
		downloaded, cleanup, err := prepareRemoteStatsFile(entity)
		if err != nil {
			return "", "", "", nil, remoteEntity, err
		}
		return entity, localFile, downloaded, cleanup, remoteEntity, nil
	}
	entity = normalizeLocalEntityPath(entity)
	entity = modifyLocalFileEntity(entity)
	return entity, localFile, entity, nil, remoteEntity, nil
}

func normalizeCategoryForSend(category string, explicit bool) string {
	if explicit && (category == "coding" || category == "null") {
		return ""
	}
	return category
}

func categoryForHeartbeat(entity string, o Options) string {
	if o.Category != "" {
		return normalizeCategoryForSend(o.Category, o.CategorySet)
	}
	if o.EntityType != "file" {
		return ""
	}
	return detectWakaTimeCategory(entity)
}

func detectWakaTimeCategory(entity string) string {
	file := strings.ToLower(filepath.ToSlash(entity))
	if strings.HasSuffix(file, "_test.go") {
		return "writing tests"
	}
	for _, part := range []string{"/tests/", "/test/", "/testdata/", "/spec/", "/specs/"} {
		if strings.Contains(file, part) {
			return "writing tests"
		}
	}
	if regexp.MustCompile(`(?i).*[\.\-_](test|spec)\.[^./\\]+$`).MatchString(file) {
		return "writing tests"
	}
	if strings.HasSuffix(file, ".md") || strings.HasSuffix(file, ".mdx") {
		return "writing docs"
	}
	return ""
}

type heartbeatSanitizeInput struct {
	hideBranchNames   string
	hideDependencies  string
	hideFileNames     string
	hideProjectFolder bool
	hideProjectNames  string
	projectRoot       string
	remoteEntity      bool
}

func sanitizeHeartbeat(hb Heartbeat, input heartbeatSanitizeInput) Heartbeat {
	if len(hb.Dependencies) == 0 {
		hb.Dependencies = nil
	}
	projectRoot := sanitizeProjectRoot(hb, input)
	if shouldSanitizeHeartbeat(hb, projectRoot, input.hideProjectNames) {
		hb.Project = obfuscatedProjectName(projectRoot)
		hb = sanitizeHeartbeatMetadata(hb)
	}
	fileSanitized := shouldSanitizeHeartbeat(hb, input.projectRoot, input.hideFileNames)
	if fileSanitized {
		if hb.EntityType == "file" && !input.remoteEntity {
			hb.Entity = "HIDDEN" + filepath.Ext(hb.Entity)
		} else {
			hb.Entity = "HIDDEN"
		}
		if len(splitConfigList(input.hideBranchNames)) == 0 {
			hb.Branch = ""
		}
		if len(splitConfigList(input.hideDependencies)) == 0 {
			hb.Dependencies = nil
		}
		hb = sanitizeHeartbeatMetadata(hb)
	}
	if hb.Branch != "" && shouldSanitizeHeartbeat(hb, input.projectRoot, input.hideBranchNames) {
		hb.Branch = ""
	}
	if hb.Dependencies != nil && shouldSanitizeHeartbeat(hb, input.projectRoot, input.hideDependencies) {
		hb.Dependencies = nil
	}
	if input.hideProjectFolder && hb.EntityType == "file" && !input.remoteEntity {
		if input.projectRoot != "" && strings.HasPrefix(hb.Entity, input.projectRoot) {
			if rel, err := filepath.Rel(input.projectRoot, hb.Entity); err == nil {
				hb.Entity = rel
				hb.ProjectRootCount = nil
			}
		} else {
			hb.Entity = filepath.Base(hb.Entity)
			hb.ProjectRootCount = nil
		}
	}
	hb.Entity = hideRemoteCredentials(hb.Entity)
	return hb
}

func sanitizeHeartbeatMetadata(hb Heartbeat) Heartbeat {
	hb.CursorPosition = nil
	hb.LineNumber = nil
	hb.Lines = nil
	hb.ProjectRootCount = nil
	return hb
}

func sanitizeProjectRoot(hb Heartbeat, input heartbeatSanitizeInput) string {
	if strings.TrimSpace(input.projectRoot) != "" {
		return input.projectRoot
	}
	if hb.EntityType == "file" && !input.remoteEntity && hb.Entity != "" {
		return filepath.Dir(hb.Entity)
	}
	return ""
}

func obfuscatedProjectName(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	projectFile := filepath.Join(root, ".wakatime-project")
	if data, err := os.ReadFile(projectFile); err == nil {
		name := strings.TrimSpace(strings.SplitN(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n", 2)[0])
		if name != "" {
			return strings.ReplaceAll(name, "{project}", filepath.Base(root))
		}
		return ""
	}
	name := "hidden-" + uuid.NewString()
	if err := os.WriteFile(projectFile, []byte(name+"\n"), 0o644); err != nil {
		return ""
	}
	return name
}

func shouldSanitizeHeartbeat(hb Heartbeat, root, rawPatterns string) bool {
	for _, pattern := range splitConfigList(rawPatterns) {
		re, err := compileWakaPattern(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(hb.Entity) || re.MatchString(root) {
			return true
		}
	}
	return false
}

func hideRemoteCredentials(entity string) string {
	if !isRemoteEntity(entity) {
		return entity
	}
	parsed, err := url.Parse(entity)
	if err != nil || parsed.User == nil {
		return entity
	}
	parsed.User = nil
	return parsed.String()
}

func isRemoteEntity(entity string) bool {
	return remoteEntityRegex.MatchString(strings.TrimSpace(entity))
}

func normalizeLocalEntityPath(entity string) string {
	entity = expandHome(entity)
	if !filepath.IsAbs(entity) {
		if abs, err := filepath.Abs(entity); err == nil {
			entity = abs
		}
	}
	if realPath, err := filepath.EvalSymlinks(entity); err == nil {
		return realPath
	}
	return entity
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

func parseMetadata(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return map[string]any{"value": raw}
	}
	return metadata
}

func projectRootCount(root string) *int {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	clean := formatProjectRootForSlashCount(root)
	if !strings.HasSuffix(clean, "/") {
		clean += "/"
	}
	count := strings.Count(clean, "/")
	return &count
}

func formatProjectRootForSlashCount(root string) string {
	isWindowsNetworkMount := strings.HasPrefix(root, `\\`)
	clean := filepath.Clean(root)
	clean = pathSeparatorRunsRegex.ReplaceAllString(clean, "/")
	if isWindowsNetworkMount && strings.HasPrefix(clean, "/") {
		clean = `\\` + clean[1:]
	}
	return clean
}

func detectProject(entity string, o Options) (project, branch, root string) {
	if entity != "" {
		root = findProjectRoot(filepath.Dir(entity))
	}
	project, branch = readWakaTimeProjectFile(root)
	if project == "" && o.ProjectFolder != "" {
		projectRoot := normalizeLocalEntityPath(o.ProjectFolder)
		if overrideProject, overrideBranch := readWakaTimeProjectFile(projectRoot); overrideProject != "" {
			project = overrideProject
			branch = overrideBranch
			root = projectRoot
		} else if root == "" {
			root = projectRoot
		}
	}
	if project == "" {
		for _, entry := range o.Config.OrderedSection("projectmap") {
			re, err := compileWakaPattern(entry.Key)
			if err != nil {
				continue
			}
			if matches := re.FindStringSubmatch(entity); len(matches) > 0 {
				project = expandProjectMap(entry.Value, root, matches[1:])
				break
			}
		}
	}
	if project == "" && root != "" {
		project = filepath.Base(root)
		if parseBoolLike(o.Config.Get("git", "project_from_git_remote")) {
			if remoteProject := gitRemoteProject(root); remoteProject != "" {
				project = remoteProject
			}
		}
	}
	if root != "" && strings.Contains(project, "{project}") {
		replacement := filepath.Base(root)
		if parseBoolLike(o.Config.Get("git", "project_from_git_remote")) {
			if remoteProject := gitRemoteProject(root); remoteProject != "" {
				replacement = remoteProject
			}
		}
		project = strings.ReplaceAll(project, "{project}", replacement)
	}
	if root != "" {
		if submodule, ok := detectGitSubmodule(root, entity, o); ok {
			if o.Project == "" && submodule.project != "" {
				project = submodule.project
			}
			if branch == "" {
				branch = submodule.branch
			}
		}
	}
	if branch == "" && root != "" {
		switch detectVCS(root) {
		case "git":
			branch = commandOutput(root, "git", "rev-parse", "--abbrev-ref", "HEAD")
			if branch == "HEAD" {
				branch = commandOutput(root, "git", "rev-parse", "--short", "HEAD")
			}
		case "hg":
			branch = hgBranch(root)
		case "svn":
			branch = svnBranch(root)
		}
	}
	return project, branch, root
}

func readWakaTimeProjectFile(root string) (project, branch string) {
	projectFile := filepath.Join(root, ".wakatime-project")
	if root == "" || !fileExists(projectFile) {
		return "", ""
	}
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return "", ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	project = strings.TrimSpace(lines[0])
	if project == "" {
		project = filepath.Base(root)
	}
	if len(lines) > 1 {
		branch = strings.TrimSpace(lines[1])
	}
	return project, branch
}

type gitSubmoduleInfo struct {
	project string
	branch  string
}

func detectGitSubmodule(root, entity string, o Options) (gitSubmoduleInfo, bool) {
	gitFile := filepath.Join(root, ".git")
	info, err := os.Stat(gitFile)
	if err != nil || info.IsDir() {
		return gitSubmoduleInfo{}, false
	}
	gitdir := readGitdirFile(gitFile)
	if gitdir == "" || !strings.Contains(filepath.ToSlash(gitdir), "/modules/") {
		return gitSubmoduleInfo{}, false
	}
	if gitSubmoduleDisabled(root, entity, o.Config.Get("git", "submodules_disabled")) {
		return gitSubmoduleInfo{}, false
	}
	project := filepath.Base(gitdir)
	for _, entry := range o.Config.OrderedSection("git_submodule_projectmap") {
		re, err := compileWakaPattern(entry.Key)
		if err != nil {
			continue
		}
		if matches := re.FindStringSubmatch(gitdir); len(matches) > 0 {
			project = expandProjectMap(entry.Value, root, matches[1:])
			break
		}
		if matches := re.FindStringSubmatch(root); len(matches) > 0 {
			project = expandProjectMap(entry.Value, root, matches[1:])
			break
		}
	}
	return gitSubmoduleInfo{
		project: project,
		branch:  gitBranchFromHead(filepath.Join(gitdir, "HEAD")),
	}, true
}

func readGitdirFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if !strings.HasPrefix(line, "gitdir:") {
		return ""
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Clean(filepath.Join(filepath.Dir(path), gitdir))
	}
	if !fileExists(filepath.Join(gitdir, "HEAD")) {
		return ""
	}
	return gitdir
}

func gitRemoteProject(root string) string {
	gitPath := filepath.Join(root, ".git")
	gitDir := gitPath
	if info, err := os.Stat(gitPath); err == nil && !info.IsDir() {
		gitDir = readGitdirFile(gitPath)
	}
	if gitDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	inOrigin := false
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if !inOrigin || !strings.HasPrefix(line, "url") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if project := projectFromRemoteURL(strings.TrimSpace(parts[1])); project != "" {
			return project
		}
	}
	return ""
}

func projectFromRemoteURL(remote string) string {
	remote = strings.TrimSpace(strings.TrimSuffix(remote, ".git"))
	if remote == "" {
		return ""
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Scheme != "" {
		return strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	}
	if i := strings.Index(remote, ":"); i >= 0 && i+1 < len(remote) {
		return strings.Trim(strings.TrimSuffix(remote[i+1:], ".git"), "/")
	}
	return strings.Trim(remote, "/")
}

func gitSubmoduleDisabled(root, entity, raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if parseBoolLike(raw) {
		return true
	}
	switch strings.ToLower(raw) {
	case "0", "false", "no", "off":
		return false
	}
	for _, pattern := range splitConfigList(raw) {
		re, err := compileWakaPattern(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(root) || re.MatchString(entity) {
			return true
		}
	}
	return false
}

func splitConfigList(raw string) []string {
	fields := strings.FieldsFunc(strings.ReplaceAll(raw, "\r", "\n"), func(r rune) bool {
		return r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func expandProjectMap(value, root string, captures []string) string {
	value = strings.TrimSpace(value)
	if root != "" {
		value = strings.ReplaceAll(value, "{project}", filepath.Base(root))
	}
	for i, capture := range captures {
		value = strings.ReplaceAll(value, fmt.Sprintf("{%d}", i), capture)
	}
	return value
}

func findProjectRoot(dir string) string {
	for dir != "" && dir != "." && dir != string(filepath.Separator) {
		if fileExists(filepath.Join(dir, ".wakatime-project")) ||
			fileExists(filepath.Join(dir, ".git")) ||
			fileExists(filepath.Join(dir, ".hg")) ||
			fileExists(filepath.Join(dir, ".svn")) {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectVCS(root string) string {
	switch {
	case fileExists(filepath.Join(root, ".git")):
		return "git"
	case fileExists(filepath.Join(root, ".hg")):
		return "hg"
	case fileExists(filepath.Join(root, ".svn")):
		return "svn"
	default:
		return ""
	}
}

func hgBranch(root string) string {
	data, err := os.ReadFile(filepath.Join(root, ".hg", "branch"))
	if err != nil {
		return "default"
	}
	if branch := strings.TrimSpace(string(data)); branch != "" {
		return branch
	}
	return "default"
}

func gitBranchFromHead(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(head, prefix) {
		return strings.TrimPrefix(head, prefix)
	}
	if len(head) >= 7 {
		return head[:7]
	}
	return head
}

func svnBranch(root string) string {
	out := commandOutput(root, "svn", "info", root)
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if ok && key == "URL" {
			parts := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' })
			if len(parts) > 0 {
				return strings.TrimSpace(parts[len(parts)-1])
			}
		}
	}
	return ""
}

func commandOutput(dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectLanguage(path string) string {
	return detectLanguageWithGuess(path, false)
}

func detectLanguageWithGuess(path string, guess bool) string {
	base := filepath.Base(path)
	if language := wakaTimeExactLanguageFiles[strings.ToLower(base)]; language != "" {
		return language
	}
	if strings.EqualFold(base, "go.mod") {
		return "Go"
	}
	if base == "CMmakeLists.txt" {
		return "CMake"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if language := wakaTimeExtensionLanguages[ext]; language != "" {
		return language
	}
	switch ext {
	case ".go":
		return "Go"
	case ".js":
		return "JavaScript"
	case ".jsx":
		return "JSX"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".py":
		return "Python"
	case ".rb":
		return "Ruby"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c", ".h":
		return detectCFamilyLanguage(path)
	case ".cc", ".cpp", ".hpp":
		return "C++"
	case ".m":
		return detectMFileLanguage(path)
	case ".mm":
		if siblingExists(strings.TrimSuffix(path, filepath.Ext(path)), ".h") {
			return "Objective-C++"
		}
		return "Objective-C++"
	case ".pas":
		if folderContainsAnyExtension(filepath.Dir(path), ".fmx", ".dfm", ".dproj") {
			return "Delphi"
		}
		if lexer := lexers.Match(path); lexer != nil {
			return lexer.Config().Name
		}
		return ""
	case ".cs":
		return "C#"
	case ".php":
		return "PHP"
	case ".sh", ".bash", ".zsh":
		return "Bash"
	case ".fs":
		return detectFSFileLanguage(path)
	case ".swift":
		return "Swift"
	case ".pbxproj":
		return "Xcode Config"
	case ".md":
		return "Markdown"
	default:
		if lexer := lexers.Match(path); lexer != nil {
			return wakaTimeLanguageName(lexer.Config().Name)
		}
		if guess {
			head := readHead(path, 64*1024)
			if language := detectVimModelineLanguage(head); language != "" {
				return language
			}
			if lexer := lexers.Analyse(head); lexer != nil {
				return wakaTimeLanguageName(lexer.Config().Name)
			}
		}
		return ""
	}
}

func detectFSFileLanguage(path string) string {
	text := readHead(path, 64*1024)
	forthWeight := float32(0)
	if forthFuncRegex.MatchString(text) {
		forthWeight = 0.9
	}
	if strings.Contains(text, `\ `) {
		forthWeight += 0.5
	}
	if strings.Contains(text, "( ") {
		forthWeight += 0.2
	}
	if forthWeight > 1 {
		forthWeight = 1
	}

	fsharpWeight := float32(0)
	if strings.Contains(text, "let ") && strings.Contains(text, "match ") && strings.Contains(text, " ->") {
		fsharpWeight = 0.9
	}
	if strings.Contains(text, "// ") || (strings.Contains(text, "(* ") && strings.Contains(text, " *)")) {
		fsharpWeight += 0.7
	}
	if fsharpWeight > 1 {
		fsharpWeight = 1
	}

	if fsharpWeight > 0 && fsharpWeight >= forthWeight {
		return "F#"
	}
	if forthWeight > 0 {
		return "Forth"
	}
	if lexer := lexers.Match(path); lexer != nil {
		return wakaTimeLanguageName(lexer.Config().Name)
	}
	return ""
}

func detectCFamilyLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	if ext == ".h" || strings.HasPrefix(ext, ".c") {
		if siblingExists(stem, ".c") {
			return "C"
		}
		if siblingExists(stem, ".m") {
			return "Objective-C"
		}
		if siblingExists(stem, ".mm") {
			return "Objective-C++"
		}
		dir := filepath.Dir(path)
		if folderContainsAnyExtension(dir, ".cpp", ".hpp", ".c++", ".h++", ".cc", ".hh", ".cxx", ".hxx", ".C", ".H", ".cp", ".CPP") {
			return "C++"
		}
		if folderContainsAnyExtension(dir, ".c") {
			return "C"
		}
	}
	return "C"
}

func detectMFileLanguage(path string) string {
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	if siblingExists(stem, ".h") {
		return "Objective-C"
	}
	if folderContainsAnyExtension(filepath.Dir(path), ".mat") {
		return "Matlab"
	}
	if lexer := lexers.Match(path); lexer != nil {
		return wakaTimeLanguageName(lexer.Config().Name)
	}
	return ""
}

func siblingExists(stem, extension string) bool {
	if _, err := os.Stat(stem + extension); err == nil {
		return true
	}
	if _, err := os.Stat(stem + strings.ToUpper(extension)); err == nil {
		return true
	}
	return false
}

func folderContainsAnyExtension(dir string, extensions ...string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		for _, candidate := range extensions {
			if ext == candidate {
				return true
			}
		}
	}
	return false
}

func detectVimModelineLanguage(text string) string {
	matches := vimModelineRegex.FindStringSubmatch(text)
	if len(matches) != 2 {
		return ""
	}
	name := strings.ToLower(strings.TrimSpace(matches[1]))
	switch name {
	case "a65", "asm", "asm68k", "asmh8300":
		name = "asm"
	case "cs":
		name = "csharp"
	case "htmlcheetah", "htmldjango", "htmlm4", "xhtml":
		name = "html"
	case "lhaskell":
		name = "haskell"
	case "objc":
		return "Objective-C"
	case "objcpp":
		return "Objective-C++"
	case "perl6":
		name = "perl"
	case "phtml":
		name = "php"
	case "vb":
		return "VB.NET"
	case "vim":
		return "VimL"
	}
	if lexer := lexers.Get(name); lexer != nil {
		return wakaTimeLanguageName(lexer.Config().Name)
	}
	return ""
}

func wakaTimeLanguageName(name string) string {
	switch strings.ToLower(name) {
	case "emacslisp":
		return "Emacs Lisp"
	case "fsharp":
		return "F#"
	case "markdown":
		return "Markdown"
	case "plaintext":
		return "Text"
	case "r":
		return "S"
	case "reasonml":
		return "Reason"
	case "systemverilog":
		return "SystemVerilog"
	case "vue":
		return "Vue.js"
	default:
		return name
	}
}

func readHead(path string, maxBytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return ""
	}
	return string(data)
}

func countLines(path string) int {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxFileStatsBytes {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func excluded(entity string, includes, excludes []string) (bool, error) {
	for _, pattern := range includes {
		re, err := compileWakaPattern(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(entity) {
			return false, nil
		}
	}
	for _, pattern := range excludes {
		re, err := compileWakaPattern(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(entity) {
			return true, nil
		}
	}
	return false, nil
}

type wakaPattern struct {
	re  *regexp.Regexp
	re2 *regexp2.Regexp
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

func compileWakaPattern(pattern string) (wakaPattern, error) {
	pattern = strings.TrimSpace(pattern)
	switch strings.ToLower(pattern) {
	case "":
		pattern = "a^"
	case "true":
		pattern = "(?s).*"
	case "false":
		pattern = "a^"
	}
	if !strings.HasPrefix(pattern, "(?i)") {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err == nil {
		return wakaPattern{re: re}, nil
	}
	re2, err := regexp2.Compile(pattern, 0)
	if err != nil {
		return wakaPattern{}, err
	}
	re2.MatchTimeout = wakaRegexMatchTimeout
	return wakaPattern{re2: re2}, nil
}

func machineName(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if gitpod := strings.TrimSpace(os.Getenv("GITPOD_WORKSPACE_ID")); gitpod != "" {
		return "Gitpod"
	}
	name, _ := os.Hostname()
	return name
}
