package stintcli

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	if o.ProjectFolder != "" {
		projectRoot := normalizeLocalEntityPath(o.ProjectFolder)
		projectFolderContainsEntity := entity == "" || pathWithin(projectRoot, entity)
		projectRootIsMoreSpecific := root == "" || (projectFolderContainsEntity && pathWithin(root, projectRoot) && root != projectRoot)
		if overrideProject, overrideBranch := readWakaTimeProjectFile(projectRoot); overrideProject != "" && (project == "" || projectRootIsMoreSpecific) {
			project = overrideProject
			branch = overrideBranch
			root = projectRoot
		} else if project == "" || projectRootIsMoreSpecific {
			project = ""
			branch = ""
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
	tempRoot := filepath.Clean(os.TempDir())
	for dir != "" && dir != "." && dir != string(filepath.Separator) {
		if filepath.Clean(dir) == tempRoot {
			break
		}
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

func pathWithin(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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
