package stintcli

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

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
