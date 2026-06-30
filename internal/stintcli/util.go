package stintcli

import (
	"os"
	"path/filepath"
	"strings"
)

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBoolLike(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func wakaResourcesDir() string {
	if home := strings.TrimSpace(os.Getenv("WAKATIME_HOME")); home != "" {
		return expandHome(home)
	}
	return expandHome(filepath.Join("~", ".wakatime"))
}
