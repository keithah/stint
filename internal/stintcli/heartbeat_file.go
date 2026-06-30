package stintcli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	remoteEntityRegex       = regexp.MustCompile(`(?i)^(ssh|sftp)://`)
	testSpecFileEntityRegex = regexp.MustCompile(`(?i).*[\.\-_](test|spec)\.[^./\\]+$`)
)

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
	if testSpecFileEntityRegex.MatchString(file) {
		return "writing tests"
	}
	if strings.HasSuffix(file, ".md") || strings.HasSuffix(file, ".mdx") {
		return "writing docs"
	}
	return ""
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
