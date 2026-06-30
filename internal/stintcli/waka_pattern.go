package stintcli

import (
	"regexp"
	"strings"

	"github.com/dlclark/regexp2"
)

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
