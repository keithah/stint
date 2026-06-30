package stintcli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxAITranscriptFiles     = 10000
	maxAITranscriptFileBytes = 32 * 1024 * 1024
)

func normalizeAIWorkspacePath(value string) string {
	value = strings.TrimSpace(normalizeAIFileURI(value))
	if value == "" {
		return ""
	}
	value = expandHome(value)
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return filepath.Clean(value)
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func aiTextContent(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, item := range v {
			object, _ := item.(map[string]any)
			if text := firstString(object["text"]); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	default:
		return ""
	}
}

func (source aiTranscriptSource) matches(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, allowed := range source.Extensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

func parseAITranscriptFile(path, source string, after, fallbackTime time.Time) (aiTranscriptSummary, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return parseAIJSONFile(path, source, after, fallbackTime)
	default:
		return parseAIJSONLines(path, source, after, fallbackTime)
	}
}

func parseAIJSONFile(path, source string, after, fallbackTime time.Time) (aiTranscriptSummary, error) {
	data, err := readAITranscriptFile(path)
	if err != nil {
		return aiTranscriptSummary{}, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return aiTranscriptSummary{}, err
	}
	summary := aiTranscriptSummary{
		Source:       source,
		SessionID:    aiTranscriptSessionID(path),
		CWD:          aiTranscriptProjectRoot(path, source),
		LastActivity: fallbackTime,
		Files:        map[string]bool{},
		FileWrites:   map[string]bool{},
		LineChanges:  map[string]int{},
	}
	updateAISummary(&summary, value)
	if summary.LastActivity.Before(after) {
		return aiTranscriptSummary{}, nil
	}
	return summary, nil
}

func readAITranscriptFile(path string) ([]byte, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxAITranscriptFileBytes {
		return nil, fmt.Errorf("AI transcript %s is larger than %d bytes", path, maxAITranscriptFileBytes)
	}
	return os.ReadFile(path)
}

func aiTranscriptProjectRoot(path, source string) string {
	if source != "Gemini" {
		return ""
	}
	for dir := filepath.Dir(path); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		rootPath := filepath.Join(dir, ".project_root")
		data, err := os.ReadFile(filepath.Clean(rootPath))
		if err == nil {
			return normalizeAIWorkspacePath(string(data))
		}
		if filepath.Base(dir) == "tmp" {
			break
		}
	}
	return ""
}

func updateAISummary(summary *aiTranscriptSummary, value any) {
	switch v := value.(type) {
	case map[string]any:
		if ts := jsonTime(v); !ts.IsZero() && ts.After(summary.LastActivity) {
			summary.LastActivity = ts
		}
		updateAICopilotCLIMetadata(summary, v)
		for key, raw := range v {
			lower := strings.ToLower(key)
			if lower == "sessionid" || lower == "session_id" || lower == "chatsessionid" || lower == "chat_session_id" || lower == "conversationid" || lower == "conversation_id" || lower == "threadid" || lower == "thread_id" || lower == "id" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					if lower != "id" || summary.SessionID == "" {
						summary.SessionID = strings.TrimSpace(s)
					}
				}
			}
			if lower == "cwd" || lower == "workspace" || lower == "workspacedirectory" || lower == "workspace_directory" || lower == "workspaceroot" || lower == "workspace_root" || lower == "workdir" || lower == "workingdir" || lower == "working_dir" || lower == "projectpath" || lower == "project_path" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.CWD = expandHome(strings.TrimSpace(s))
				}
			}
			if lower == "model" || lower == "modelname" || lower == "model_name" || lower == "modelid" || lower == "model_id" || lower == "modelslug" || lower == "selectedmodel" || lower == "selectedchatmodel" || lower == "newmodel" || lower == "currentmodel" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.Model = strings.TrimSpace(s)
				}
			}
			if lower == "provider" || lower == "modelprovider" || lower == "model_provider" || lower == "aiprovider" || lower == "ai_provider" || lower == "llmprovider" || lower == "llm_provider" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.Provider = strings.TrimSpace(s)
				}
			}
			if lower == "subscriptionplan" || lower == "subscription_plan" || lower == "aisubscriptionplan" || lower == "ai_subscription_plan" || lower == "plan" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.SubscriptionPlan = strings.TrimSpace(s)
				}
			}
			if lower == "agent" || lower == "aiagent" || lower == "ai_agent" || lower == "agentname" || lower == "agent_name" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.Agent = strings.TrimSpace(s)
				}
			}
			if lower == "agentversion" || lower == "agent_version" || lower == "aiagentversion" || lower == "ai_agent_version" || lower == "copilotversion" || lower == "version" {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					summary.AgentVersion = strings.TrimSpace(s)
				}
			}
			if lower == "inputtokens" || lower == "input_tokens" || lower == "prompttokens" || lower == "prompt_tokens" || lower == "prompttokencount" || lower == "cachecreationinputtokens" || lower == "cache_creation_input_tokens" || lower == "cachereadinputtokens" || lower == "cache_read_input_tokens" || lower == "cacheread" || lower == "cachewrite" {
				summary.InputTokens += jsonInt(raw)
			}
			if lower == "tokensin" {
				summary.InputTokens += jsonInt(raw)
			}
			if lower == "outputtokens" || lower == "output_tokens" || lower == "completiontokens" || lower == "completion_tokens" || lower == "candidatestokencount" || lower == "reasoningoutputtokens" || lower == "reasoning_output_tokens" {
				summary.OutputTokens += jsonInt(raw)
			}
			if lower == "tokensout" {
				summary.OutputTokens += jsonInt(raw)
			}
			if lower == "input" && jsonInt(raw) > 0 {
				summary.InputTokens += jsonInt(raw)
			}
			if lower == "output" && jsonInt(raw) > 0 {
				summary.OutputTokens += jsonInt(raw)
			}
			if lower == "prompt_length" {
				summary.PromptLength += jsonInt(raw)
			}
			if isAIPathKey(lower) {
				switch value := raw.(type) {
				case string:
					paths := aiPathsFromValue(value, summary.CWD)
					for _, path := range paths {
						summary.Files[path] = true
					}
				case []any:
					for _, item := range value {
						s, ok := item.(string)
						if !ok {
							continue
						}
						for _, path := range aiPathsFromValue(s, summary.CWD) {
							summary.Files[path] = true
						}
					}
				}
			}
			if lower == "request" {
				if s, ok := raw.(string); ok {
					updateAIRooRequest(summary, s)
				}
			}
			if lower == "patchtext" || lower == "patch_text" {
				if s, ok := raw.(string); ok {
					updateAIPatchText(summary, s)
				}
			}
			if lower == "text" || lower == "input" || lower == "message" || lower == "prompt" || lower == "content" {
				if s, ok := raw.(string); ok {
					if parsed, ok := parseAIEmbeddedJSON(s); ok {
						updateAISummary(summary, parsed)
						continue
					}
					if summary.PromptLength == 0 {
						summary.PromptLength = len([]rune(aiPromptText(summary, s)))
					}
				}
			}
			if lower == "content" {
				if s, ok := raw.(string); ok {
					for _, path := range aiContentPaths(s, summary.CWD) {
						summary.Files[path] = true
					}
				}
			}
			updateAISummary(summary, raw)
		}
		updateAICodexPatch(summary, v)
		updateAIToolLineChanges(summary, v)
	case []any:
		for _, item := range v {
			updateAISummary(summary, item)
		}
	}
}

func aiPromptText(summary *aiTranscriptSummary, raw string) string {
	prompt := strings.TrimSpace(raw)
	if strings.EqualFold(summary.Source, "Codex") {
		return cleanCodexPrompt(prompt)
	}
	if strings.EqualFold(summary.Source, "Claude") {
		return cleanClaudePrompt(prompt)
	}
	return prompt
}

func cleanCodexPrompt(prompt string) string {
	prompt = strings.ReplaceAll(prompt, "\r\n", "\n")
	prompt = strings.TrimSpace(prompt)
	for _, marker := range []string{
		"## My request for Codex:",
		"## My request for Codex",
		"My request for Codex:",
		"User request:",
	} {
		if idx := strings.LastIndex(prompt, marker); idx >= 0 {
			return strings.TrimSpace(prompt[idx+len(marker):])
		}
	}
	if idx := strings.LastIndex(prompt, "</environment_context>"); idx >= 0 {
		return strings.TrimSpace(prompt[idx+len("</environment_context>"):])
	}
	return prompt
}

func cleanClaudePrompt(prompt string) string {
	prompt = strings.ReplaceAll(prompt, "\r\n", "\n")
	prompt = claudeSystemReminderRe.ReplaceAllString(prompt, "")
	return strings.TrimSpace(prompt)
}

func updateAIPatchText(summary *aiTranscriptSummary, patch string) {
	for path, lineChanges := range aiPatchLineChanges(patch, summary.CWD) {
		summary.Files[path] = true
		summary.FileWrites[path] = true
		summary.LineChanges[path] = lineChanges
	}
}

func updateAICodexPatch(summary *aiTranscriptSummary, payload map[string]any) {
	if !strings.EqualFold(summary.Source, "Codex") {
		return
	}
	payloadType := firstString(payload["type"])
	callID := firstString(payload["call_id"], payload["callId"])
	if payloadType == "custom_tool_call" && callID != "" {
		patch := codexPatchFromToolCall(firstString(payload["name"]), firstString(payload["input"]))
		if patch == "" {
			return
		}
		if summary.PendingPatch == nil {
			summary.PendingPatch = map[string]string{}
		}
		summary.PendingPatch[callID] = patch
		return
	}
	if !codexPatchCompletion(payloadType) || callID == "" {
		return
	}
	patch := ""
	if summary.PendingPatch != nil {
		patch = summary.PendingPatch[callID]
		delete(summary.PendingPatch, callID)
	}
	if patch == "" || !codexPatchSucceeded(payload) {
		return
	}
	updateAIPatchText(summary, patch)
}

func codexPatchFromToolCall(name, input string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "apply_patch":
		return input
	case "exec", "exec_command", "shell", "shell_command":
		return codexPatchFromShellInput(input)
	default:
		return ""
	}
}

func codexPatchFromShellInput(input string) string {
	command := input
	if parsed, ok := parseAIEmbeddedJSON(input); ok {
		if object, ok := parsed.(map[string]any); ok {
			command = firstString(object["cmd"], object["command"], object["script"])
		}
	}
	if !strings.Contains(command, "apply_patch") {
		return ""
	}
	start := strings.Index(command, "*** Begin Patch")
	end := strings.LastIndex(command, "*** End Patch")
	if start < 0 || end < start {
		return ""
	}
	end += len("*** End Patch")
	return strings.TrimSpace(command[start:end])
}

func codexPatchCompletion(payloadType string) bool {
	switch payloadType {
	case "patch_apply_end", "exec_command_end", "exec_end", "shell_command_end", "command_end":
		return true
	default:
		return false
	}
}

func codexPatchSucceeded(payload map[string]any) bool {
	if _, ok := payload["success"]; ok {
		return jsonBool(payload["success"])
	}
	for _, key := range []string{"exit_code", "exitCode", "code"} {
		if raw, ok := payload[key]; ok {
			return jsonInt(raw) == 0
		}
	}
	status := strings.ToLower(firstString(payload["status"], payload["result"]))
	return status == "" || status == "success" || status == "succeeded" || status == "completed" || status == "ok"
}

func aiPatchLineChanges(patch, cwd string) map[string]int {
	changes := map[string]int{}
	currentPath := ""
	for _, line := range strings.Split(patch, "\n") {
		if path, ok := aiPatchFilePath(line, cwd); ok {
			currentPath = path
			if _, exists := changes[currentPath]; !exists {
				changes[currentPath] = 0
			}
			continue
		}
		if currentPath == "" || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			changes[currentPath]++
		case strings.HasPrefix(line, "-"):
			changes[currentPath]--
		}
	}
	return changes
}

func aiPatchFilePath(line, cwd string) (string, bool) {
	for _, prefix := range []string{"*** Update File:", "*** Add File:"} {
		if strings.HasPrefix(line, prefix) {
			path := normalizeAIPath(strings.TrimSpace(strings.TrimPrefix(line, prefix)), cwd)
			return path, path != ""
		}
	}
	return "", false
}

func updateAICopilotCLIMetadata(summary *aiTranscriptSummary, payload map[string]any) {
	eventType := strings.ToLower(firstString(payload["type"]))
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return
	}
	if eventType == "session.start" {
		summary.Agent = "github-copilot-cli"
	}
	if eventType == "session.shutdown" {
		addAITokenDetails(summary, data)
	}
	telemetry, _ := data["toolTelemetry"].(map[string]any)
	if telemetry == nil {
		return
	}
	paths := aiToolTelemetryPaths(telemetry, summary.CWD)
	if len(paths) == 0 {
		return
	}
	lineChanges, hasLineChanges := aiToolTelemetryLineChanges(telemetry)
	for _, path := range paths {
		summary.Files[path] = true
		summary.FileWrites[path] = true
		if hasLineChanges {
			summary.LineChanges[path] = lineChanges
		}
	}
}

func addAITokenDetails(summary *aiTranscriptSummary, data map[string]any) {
	tokenDetails, _ := data["tokenDetails"].(map[string]any)
	if tokenDetails == nil {
		return
	}
	if input, ok := tokenDetails["input"].(map[string]any); ok {
		summary.InputTokens += jsonInt(input["tokenCount"])
	}
	if output, ok := tokenDetails["output"].(map[string]any); ok {
		summary.OutputTokens += jsonInt(output["tokenCount"])
	}
}

func aiToolTelemetryPaths(telemetry map[string]any, cwd string) []string {
	restricted, _ := telemetry["restrictedProperties"].(map[string]any)
	if restricted == nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"filePaths", "addedPaths", "deletedPaths"} {
		paths = append(paths, aiPathsFromAny(restricted[key], cwd)...)
	}
	return paths
}

func aiToolTelemetryLineChanges(telemetry map[string]any) (int, bool) {
	metrics, _ := telemetry["metrics"].(map[string]any)
	if metrics == nil {
		return 0, false
	}
	added := jsonInt(metrics["linesAdded"])
	removed := jsonInt(metrics["linesRemoved"])
	if added == 0 && removed == 0 {
		return 0, false
	}
	return added - removed, true
}

func aiPathsFromAny(value any, cwd string) []string {
	switch v := value.(type) {
	case string:
		return aiPathsFromValue(v, cwd)
	case []any:
		var paths []string
		for _, item := range v {
			paths = append(paths, aiPathsFromAny(item, cwd)...)
		}
		return paths
	default:
		return nil
	}
}

func aiPathsFromValue(value, cwd string) []string {
	if path := normalizeAIPath(value, cwd); path != "" {
		return []string{path}
	}
	if parsed, ok := parseAIEmbeddedJSON(value); ok {
		return aiPathsFromAny(parsed, cwd)
	}
	return nil
}

func parseAIEmbeddedJSON(value string) (any, bool) {
	value = strings.TrimSpace(value)
	if value == "" || (value[0] != '{' && value[0] != '[') {
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

func updateAIRooRequest(summary *aiTranscriptSummary, request string) {
	if cwd := rooRequestCWD(request); cwd != "" {
		summary.CWD = expandHome(cwd)
	}
	if task := rooRequestTask(request); task != "" && summary.PromptLength == 0 {
		summary.PromptLength = len(task)
	}
}

func rooRequestTask(request string) string {
	return betweenTrimmed(request, "<task>", "</task>")
}

func rooRequestCWD(request string) string {
	const prefix = "# Current Working Directory ("
	start := strings.Index(request, prefix)
	if start < 0 {
		return ""
	}
	rest := request[start+len(prefix):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func betweenTrimmed(value, start, end string) string {
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		return ""
	}
	rest := value[startIndex+len(start):]
	endIndex := strings.Index(rest, end)
	if endIndex < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:endIndex])
}

func updateAIToolLineChanges(summary *aiTranscriptSummary, payload map[string]any) {
	if functionCall, ok := payload["functionCall"].(map[string]any); ok {
		updateAIFunctionCall(summary, functionCall)
	}
	if firstString(payload["name"], payload["toolName"], payload["tool_call_name"]) != "" {
		updateAIFunctionCall(summary, payload)
	}
	if parameters, ok := payload["args"].(map[string]any); ok {
		if patchText := firstString(parameters["patchText"], parameters["patch_text"]); patchText != "" {
			cwd := firstString(parameters["workdir"], parameters["cwd"])
			if cwd == "" {
				cwd = summary.CWD
			}
			for path, lineChanges := range aiPatchLineChanges(patchText, cwd) {
				summary.Files[path] = true
				summary.FileWrites[path] = true
				summary.LineChanges[path] = lineChanges
			}
		}
	}
	path := firstString(payload["path"], payload["file_path"], payload["filePath"])
	if path == "" {
		if parameters, ok := payload["parameters"].(map[string]any); ok {
			path = firstString(parameters["path"], parameters["file_path"], parameters["filePath"])
		}
	}
	normalized := normalizeAIPath(path, summary.CWD)
	if normalized == "" {
		return
	}
	diff := firstString(payload["diff"])
	content := firstString(payload["content"])
	tool := strings.ToLower(firstString(payload["tool"], payload["toolCallName"], payload["tool_call_name"]))
	switch {
	case diff != "":
		summary.Files[normalized] = true
		summary.FileWrites[normalized] = true
		summary.LineChanges[normalized] = aiReplacementDiffLineChanges(diff)
	case content != "" && (strings.Contains(tool, "write") || strings.Contains(tool, "newfile") || strings.Contains(tool, "new_file") || strings.Contains(tool, "created")):
		summary.Files[normalized] = true
		summary.FileWrites[normalized] = true
		summary.LineChanges[normalized] = countAIContentLines(content)
	case strings.Contains(tool, "read"):
		summary.Files[normalized] = true
		summary.FileWrites[normalized] = false
	}
}

func updateAIFunctionCall(summary *aiTranscriptSummary, payload map[string]any) {
	tool := strings.ToLower(firstString(payload["name"], payload["toolName"], payload["tool_call_name"]))
	args, ok := payload["args"].(map[string]any)
	if !ok {
		if raw := firstString(payload["args"]); raw != "" {
			if parsed, parsedOK := parseAIEmbeddedJSON(raw); parsedOK {
				args, _ = parsed.(map[string]any)
			}
		}
	}
	if args == nil {
		return
	}
	path := normalizeAIPath(firstString(args["file_path"], args["filePath"], args["path"]), summary.CWD)
	if path == "" {
		return
	}
	summary.Files[path] = true
	switch {
	case strings.Contains(tool, "read"):
		summary.FileWrites[path] = false
		summary.LineChanges[path] = 0
	case strings.Contains(tool, "write") || strings.Contains(tool, "create"):
		summary.FileWrites[path] = true
		summary.LineChanges[path] = firstAIResultLineChanges(payload, countAIContentLines(firstString(args["content"])))
	case strings.Contains(tool, "edit") || strings.Contains(tool, "replace"):
		summary.FileWrites[path] = true
		fallback := countAIContentLines(firstString(args["new_string"], args["newString"])) - countAIContentLines(firstString(args["old_string"], args["oldString"]))
		summary.LineChanges[path] = firstAIResultLineChanges(payload, fallback)
	}
}

func firstAIResultLineChanges(payload map[string]any, fallback int) int {
	display, _ := payload["resultDisplay"].(map[string]any)
	if display == nil {
		return fallback
	}
	diffStat, _ := display["diffStat"].(map[string]any)
	if diffStat == nil {
		return fallback
	}
	added := jsonInt(firstAny(diffStat["model_added_lines"], diffStat["modelAddedLines"], diffStat["linesAdded"]))
	removed := jsonInt(firstAny(diffStat["model_removed_lines"], diffStat["modelRemovedLines"], diffStat["linesRemoved"]))
	return added - removed
}

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func aiReplacementDiffLineChanges(diff string) int {
	lines := strings.Split(diff, "\n")
	oldLines := 0
	newLines := 0
	total := 0
	state := ""
	flush := func() {
		if state == "" {
			return
		}
		total += newLines - oldLines
		oldLines = 0
		newLines = 0
		state = ""
	}
	for _, line := range lines {
		switch line {
		case "<<<<<<< SEARCH":
			flush()
			state = "old"
		case "=======":
			if state == "old" {
				state = "new"
			}
		case ">>>>>>> REPLACE":
			flush()
		default:
			switch state {
			case "old":
				oldLines++
			case "new":
				newLines++
			}
		}
	}
	flush()
	return total
}

func countAIContentLines(content string) int {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func aiTranscriptSessionID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base != "transcript" {
		return base
	}
	dir := filepath.Dir(path)
	if filepath.Base(dir) != "logs" {
		return base
	}
	sessionDir := filepath.Dir(filepath.Dir(dir))
	sessionID := filepath.Base(sessionDir)
	if sessionID == "." || sessionID == string(filepath.Separator) || strings.TrimSpace(sessionID) == "" {
		return base
	}
	return sessionID
}

func aiContentPaths(content, cwd string) []string {
	matches := antigravityCodeActionPathRe.FindAllStringSubmatch(content, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := normalizeAIPath(match[1], cwd)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func isAIPathKey(key string) bool {
	switch key {
	case "path", "paths", "fspath", "fs_path", "uri", "uris", "filepath", "filepaths", "file_path", "file_paths", "filename", "filenames", "file", "files", "originalfile", "original_file", "addedpaths", "added_paths", "deletedpaths", "deleted_paths":
		return true
	default:
		return false
	}
}

func normalizeAIPath(value, cwd string) string {
	value = strings.TrimSpace(value)
	value = normalizeAIFileURI(value)
	if value == "" || strings.Contains(value, "\n") || !aiPathRe.MatchString(value) {
		return ""
	}
	value = expandHome(value)
	if !filepath.IsAbs(value) && cwd != "" {
		value = filepath.Join(cwd, value)
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return filepath.Clean(value)
}

func normalizeAIFileURI(value string) string {
	if !strings.HasPrefix(strings.ToLower(value), "file://") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimPrefix(value, "file://")
	}
	path := parsed.Path
	if path == "" {
		path = strings.TrimPrefix(value, "file://")
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		path = string(filepath.Separator) + filepath.Join(parsed.Host, path)
	}
	return path
}
