package middleware

import (
	"regexp"
	"strings"
)

type UserAgentInfo struct {
	Plugin          string
	PluginVersion   string
	Editor          string
	EditorVersion   string
	OperatingSystem string
	Architecture    string
	AIAgent         string
	AIAgentVersion  string
}

var osArchRE = regexp.MustCompile(`\(([^)-]+)(?:-([^)]+))?\)`)

func ParseUserAgent(value string) UserAgentInfo {
	info := UserAgentInfo{Editor: "Unknown", OperatingSystem: "Unknown"}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return info
	}

	if name, version, ok := splitToken(fields[0]); ok {
		info.Plugin = name
		info.PluginVersion = version
	}
	if match := osArchRE.FindStringSubmatch(value); len(match) > 0 {
		info.OperatingSystem, info.Architecture = parseOSArch(match[0])
	}

	for _, field := range fields {
		name, version, ok := splitToken(field)
		if !ok {
			continue
		}
		switch strings.ToLower(name) {
		case "vscode", "zed", "neovim", "vim", "intellij", "goland", "pycharm", "webstorm", "cursor", "sublime", "emacs", "codex":
			info.Editor = name
			info.EditorVersion = version
		case "gpt", "claude", "gemini", "copilot", "llama", "mistral":
			info.AIAgent = name
			info.AIAgentVersion = version
		}
	}
	return info
}

func parseOSArch(token string) (string, string) {
	token = strings.TrimPrefix(strings.TrimSuffix(token, ")"), "(")
	parts := strings.Split(token, "-")
	if len(parts) == 0 || parts[0] == "" {
		return "Unknown", ""
	}
	osName := parts[0]
	if len(parts) == 1 {
		return osName, ""
	}
	arch := parts[len(parts)-1]
	if !isArchitectureToken(arch) {
		arch = strings.Join(parts[1:], "-")
	}
	return osName, arch
}

func isArchitectureToken(value string) bool {
	switch strings.ToLower(value) {
	case "386", "486", "586", "686", "amd64", "arm", "arm64", "armv6l", "armv7l", "aarch64", "i386", "i686", "x64", "x86", "x86_64":
		return true
	default:
		return false
	}
}

func splitToken(token string) (string, string, bool) {
	name, version, ok := strings.Cut(token, "/")
	if !ok || name == "" || version == "" {
		return "", "", false
	}
	return name, version, true
}
