package stintcli

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type aiSQLiteSource struct {
	Name string
	Path string
	Kind string
}

func collectAISQLiteSummaries(after time.Time) ([]aiTranscriptSummary, error) {
	var summaries []aiTranscriptSummary
	for _, source := range aiSQLiteSources() {
		paths, err := aiSQLiteSourcePaths(source)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			pathSource := source
			pathSource.Path = path
			parsed, err := collectAISQLiteSourceSummaries(pathSource, after)
			if err != nil {
				return nil, err
			}
			summaries = append(summaries, parsed...)
		}
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].LastActivity.Equal(summaries[j].LastActivity) {
			return summaries[i].Source < summaries[j].Source
		}
		return summaries[i].LastActivity.Before(summaries[j].LastActivity)
	})
	return summaries, nil
}

func aiSQLiteSourcePaths(source aiSQLiteSource) ([]string, error) {
	if source.Path == "" {
		return nil, nil
	}
	if !strings.ContainsAny(source.Path, "*?[") {
		return []string{source.Path}, nil
	}
	paths, err := filepath.Glob(source.Path)
	if err != nil {
		return nil, fmt.Errorf("glob %s sqlite sources: %w", source.Name, err)
	}
	return paths, nil
}

func collectAISQLiteSourceSummaries(source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	if source.Path == "" {
		return nil, nil
	}
	info, err := os.Stat(source.Path)
	if err != nil || info.IsDir() || info.ModTime().Before(after) {
		return nil, nil
	}
	return parseAISQLiteSource(source, after)
}

func aiSQLiteSources() []aiSQLiteSource {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []aiSQLiteSource{
		{Name: "Cursor", Kind: "cursor_disk_kv", Path: filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")},
		{Name: "Cursor", Kind: "cursor_disk_kv", Path: filepath.Join(home, "AppData", "Roaming", "Cursor", "User", "globalStorage", "state.vscdb")},
		{Name: "Cursor", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")},
		{Name: "Cursor", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "cursor", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, "Library", "Application Support", "Windsurf", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, "Library", "Application Support", "Windsurf - Next", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, "AppData", "Roaming", "Windsurf", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, "AppData", "Roaming", "Windsurf - Next", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "Windsurf", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "Windsurf - Next", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "windsurf", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "windsurf-next", "User", "globalStorage", "state.vscdb")},
		{Name: "Goose", Kind: "goose_sessions", Path: filepath.Join(home, ".local", "share", "goose", "sessions", "sessions.db")},
		{Name: "Goose", Kind: "goose_sessions", Path: filepath.Join(home, "AppData", "Roaming", "Block", "goose", "data", "sessions", "sessions.db")},
		{Name: "Qoder", Kind: "qoder_local", Path: filepath.Join(home, "Library", "Application Support", "Qoder", "SharedClientCache", "cache", "db", "local.db")},
		{Name: "Qoder", Kind: "qoder_local", Path: filepath.Join(home, "AppData", "Roaming", "Qoder", "SharedClientCache", "cache", "db", "local.db")},
		{Name: "Qoder", Kind: "qoder_local", Path: filepath.Join(home, ".config", "Qoder", "SharedClientCache", "cache", "db", "local.db")},
		{Name: "Qoder", Kind: "qoder_local", Path: filepath.Join(home, ".config", "qoder", "SharedClientCache", "cache", "db", "local.db")},
		{Name: "OpenCode", Kind: "opencode_sqlite", Path: filepath.Join(home, ".local", "share", "opencode", "opencode*.db")},
		{Name: "OpenCode", Kind: "opencode_sqlite", Path: filepath.Join(home, "Library", "Application Support", "opencode", "opencode*.db")},
		{Name: "OpenCode", Kind: "opencode_sqlite", Path: filepath.Join(home, "AppData", "Local", "opencode", "opencode*.db")},
		{Name: "OpenCode", Kind: "opencode_sqlite", Path: filepath.Join(home, "AppData", "Roaming", "opencode", "opencode*.db")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "Library", "Application Support", "Code - Insiders", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "Library", "Application Support", "VSCodium", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "AppData", "Roaming", "Code", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "AppData", "Roaming", "Code - Insiders", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "AppData", "Roaming", "VSCodium", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, ".config", "Code", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, ".config", "Code - Insiders", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, ".config", "VSCodium", "User", "globalStorage", "state.vscdb")},
	}
}

func parseAISQLiteSource(source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	db, err := openAIReadOnlySQLite(source.Path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	switch source.Kind {
	case "cursor_disk_kv":
		return parseAICursorDiskKV(db, source, after)
	case "goose_sessions":
		return parseAIGooseSessions(db, source, after)
	case "cody_item_table":
		return parseAICodyItemTable(db, source, after)
	case "qoder_local":
		return parseAIQoderLocal(db, source, after)
	case "opencode_sqlite":
		return parseAIOpenCodeSQLite(db, source, after)
	default:
		return nil, nil
	}
}

func openAIReadOnlySQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_pragma=busy_timeout(5000)")
}

func parseAICursorDiskKV(db *sql.DB, source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	if !aiSQLiteTableExists(db, "cursorDiskKV") {
		return nil, nil
	}
	rows, err := db.Query(`
SELECT CAST(key AS TEXT), CAST(value AS TEXT)
FROM cursorDiskKV
WHERE key LIKE 'bubbleId:%' AND json_valid(value)
ORDER BY rowid DESC
LIMIT 5000`)
	if err != nil {
		return nil, fmt.Errorf("query %s cursorDiskKV: %w", source.Name, err)
	}
	defer rows.Close()
	var summaries []aiTranscriptSummary
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		summary, ok := aiSummaryFromJSONString(source.Name, sqliteSessionID(key), value, after)
		if ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries, rows.Err()
}

func parseAICodyItemTable(db *sql.DB, source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	if !aiSQLiteTableExists(db, "ItemTable") {
		return nil, nil
	}
	rows, err := db.Query(`
SELECT CAST(key AS TEXT), CAST(value AS TEXT)
FROM ItemTable
WHERE key = 'sourcegraph.cody-ai' OR key LIKE '%cody%chat%'
LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("query Cody ItemTable: %w", err)
	}
	defer rows.Close()
	var summaries []aiTranscriptSummary
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if key == "sourcegraph.cody-ai" {
			summaries = append(summaries, codySummariesFromStorage(source.Name, value, after)...)
			continue
		}
		summary, ok := aiSummaryFromJSONString(source.Name, key, value, after)
		if ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries, rows.Err()
}

func codySummariesFromStorage(sourceName, value string, after time.Time) []aiTranscriptSummary {
	var storage map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &storage); err != nil {
		return nil
	}
	rawHistory := storage["cody-local-chatHistory-v2"]
	if len(rawHistory) == 0 {
		return nil
	}
	var encoded string
	if err := json.Unmarshal(rawHistory, &encoded); err == nil {
		rawHistory = json.RawMessage(encoded)
	}
	var accounts map[string]struct {
		Chat map[string]codyChat `json:"chat"`
	}
	if err := json.Unmarshal(rawHistory, &accounts); err != nil {
		return nil
	}
	var summaries []aiTranscriptSummary
	for _, account := range accounts {
		for chatID, chat := range account.Chat {
			sessionID := first(chat.ID, chatID)
			lastActivity := parseAITimeString(chat.LastInteractionTimestamp)
			if lastActivity.IsZero() || lastActivity.Before(after) {
				continue
			}
			interaction, ok := codyLastInteraction(chat.Interactions)
			if !ok {
				continue
			}
			summary := aiTranscriptSummary{
				Source:       sourceName,
				SessionID:    sessionID,
				LastActivity: lastActivity,
				Files:        map[string]bool{},
				FileWrites:   map[string]bool{},
				LineChanges:  map[string]int{},
			}
			summary.PromptLength = len([]rune(strings.TrimSpace(interaction.HumanMessage.Text)))
			if interaction.AssistantMessage != nil {
				summary.Model = interaction.AssistantMessage.Model
				summary.InputTokens = interaction.AssistantMessage.TokenUsage.PromptTokens
				summary.OutputTokens = interaction.AssistantMessage.TokenUsage.CompletionTokens
			}
			for _, item := range codyContextItems(interaction) {
				path := codyItemPath(item)
				if path == "" {
					continue
				}
				summary.Files[path] = true
				if strings.EqualFold(item.ToolName, "text_editor") {
					summary.FileWrites[path] = true
					if lineChanges, ok := codyItemLineChanges(item); ok {
						summary.LineChanges[path] = lineChanges
					}
					continue
				}
				summary.FileWrites[path] = false
				summary.LineChanges[path] = 0
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

type codyChat struct {
	ID                       string            `json:"id"`
	LastInteractionTimestamp string            `json:"lastInteractionTimestamp"`
	Interactions             []codyInteraction `json:"interactions"`
}

type codyInteraction struct {
	HumanMessage     codyMessage  `json:"humanMessage"`
	AssistantMessage *codyMessage `json:"assistantMessage"`
}

type codyMessage struct {
	Text         string            `json:"text"`
	Model        string            `json:"model"`
	TokenUsage   codyTokenUsage    `json:"tokenUsage"`
	ContextFiles []codyContextItem `json:"contextFiles"`
	Processes    []codyProcessStep `json:"processes"`
	SubMessages  []codySubMessage  `json:"subMessages"`
}

type codyTokenUsage struct {
	CompletionTokens int `json:"completionTokens"`
	PromptTokens     int `json:"promptTokens"`
}

type codyProcessStep struct {
	Items []codyContextItem `json:"items"`
}

type codySubMessage struct {
	ContextFiles []codyContextItem `json:"contextFiles"`
}

type codyContextItem struct {
	ToolName string   `json:"toolName"`
	URI      codyURI  `json:"uri"`
	Metadata []string `json:"metadata"`
}

type codyURI struct {
	Scheme string `json:"scheme"`
	Path   string `json:"path"`
	FSPath string `json:"fsPath"`
}

func codyLastInteraction(interactions []codyInteraction) (codyInteraction, bool) {
	for i := len(interactions) - 1; i >= 0; i-- {
		if strings.TrimSpace(interactions[i].HumanMessage.Text) != "" || interactions[i].AssistantMessage != nil {
			return interactions[i], true
		}
	}
	return codyInteraction{}, false
}

func codyContextItems(interaction codyInteraction) []codyContextItem {
	var items []codyContextItem
	items = append(items, interaction.HumanMessage.ContextFiles...)
	if interaction.AssistantMessage == nil {
		return items
	}
	items = append(items, interaction.AssistantMessage.ContextFiles...)
	for _, process := range interaction.AssistantMessage.Processes {
		items = append(items, process.Items...)
	}
	for _, subMessage := range interaction.AssistantMessage.SubMessages {
		items = append(items, subMessage.ContextFiles...)
	}
	return items
}

func codyItemPath(item codyContextItem) string {
	rawPath := first(item.URI.FSPath, item.URI.Path)
	if item.URI.Scheme == "file" || strings.HasPrefix(strings.ToLower(rawPath), "file://") {
		return normalizeAIPath(rawPath, "")
	}
	return ""
}

func codyItemLineChanges(item codyContextItem) (int, bool) {
	if len(item.Metadata) < 2 {
		return 0, false
	}
	return codyContentLineChanges(item.Metadata[0], item.Metadata[1]), true
}

func codyContentLineChanges(oldContent, newContent string) int {
	oldLines := strings.Split(strings.TrimRight(oldContent, "\n"), "\n")
	newLines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")
	if len(oldLines) == 1 && oldLines[0] == "" {
		oldLines = nil
	}
	if len(newLines) == 1 && newLines[0] == "" {
		newLines = nil
	}
	if len(oldLines) != len(newLines) {
		return len(newLines) - len(oldLines)
	}
	changes := 0
	for i := range oldLines {
		if oldLines[i] != newLines[i] {
			changes += 2
		}
	}
	return changes
}

func parseAIGooseSessions(db *sql.DB, source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	if !aiSQLiteTableExists(db, "sessions") {
		return nil, nil
	}
	cols, err := aiSQLiteTableColumns(db, "sessions")
	if err != nil {
		return nil, err
	}
	idCol := aiSQLiteFirstPresent(cols, "id", "session_id")
	if idCol == "" {
		return nil, nil
	}
	timeCol := aiSQLiteFirstPresent(cols, "updated_at", "updatedAt", "mtime", "last_updated")
	if timeCol == "" {
		return nil, nil
	}
	nameCol := aiSQLiteFirstPresent(cols, "name", "title", "prompt")
	cwdCol := aiSQLiteFirstPresent(cols, "working_dir", "workingDir", "cwd", "workspace")
	inputCol := aiSQLiteFirstPresent(cols, "total_input_tokens", "input_tokens", "prompt_tokens")
	outputCol := aiSQLiteFirstPresent(cols, "total_output_tokens", "output_tokens", "completion_tokens")
	query := "SELECT " + strings.Join([]string{
		aiSQLiteSelectExpr(idCol),
		aiSQLiteSelectExpr(nameCol),
		aiSQLiteSelectExpr(cwdCol),
		aiSQLiteSelectExpr(timeCol),
		aiSQLiteSelectExpr(inputCol),
		aiSQLiteSelectExpr(outputCol),
	}, ", ") + " FROM sessions ORDER BY " + aiSQLiteQuoteIdent(timeCol) + " DESC LIMIT 1000"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query Goose sessions: %w", err)
	}
	defer rows.Close()
	var summaries []aiTranscriptSummary
	for rows.Next() {
		var id, name, cwd, updatedAt sql.NullString
		var input, output sql.NullInt64
		if err := rows.Scan(&id, &name, &cwd, &updatedAt, &input, &output); err != nil {
			return nil, err
		}
		ts := parseAITimeString(aiSQLString(updatedAt))
		if ts.IsZero() || ts.Before(after) {
			continue
		}
		summary := aiTranscriptSummary{
			Source:       source.Name,
			SessionID:    first(aiSQLString(id), aiSQLString(name)),
			CWD:          expandHome(aiSQLString(cwd)),
			LastActivity: ts,
			InputTokens:  aiSQLInt(input),
			OutputTokens: aiSQLInt(output),
			PromptLength: len(aiSQLString(name)),
			Files:        map[string]bool{},
			FileWrites:   map[string]bool{},
			LineChanges:  map[string]int{},
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func parseAIQoderLocal(db *sql.DB, source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	if !aiSQLiteTableExists(db, "chat_message") || !aiSQLiteTableExists(db, "chat_session") {
		return nil, nil
	}
	messageCols, err := aiSQLiteTableColumns(db, "chat_message")
	if err != nil {
		return nil, err
	}
	sessionCols, err := aiSQLiteTableColumns(db, "chat_session")
	if err != nil {
		return nil, err
	}
	sessionCol := aiSQLiteFirstPresent(messageCols, "session_id", "sessionId")
	roleCol := aiSQLiteFirstPresent(messageCols, "role")
	timeCol := aiSQLiteFirstPresent(messageCols, "gmt_create", "created_at", "createdAt", "timestamp")
	if sessionCol == "" || roleCol == "" || timeCol == "" {
		return nil, nil
	}
	requestCol := aiSQLiteFirstPresent(messageCols, "request_id", "requestId")
	toolResultCol := aiSQLiteFirstPresent(messageCols, "tool_result", "toolResult")
	tokenInfoCol := aiSQLiteFirstPresent(messageCols, "token_info", "tokenInfo")
	contentCol := aiSQLiteFirstPresent(messageCols, "content", "message", "text")
	projectCol := aiSQLiteFirstPresent(sessionCols, "project_uri", "projectUri", "project_path", "projectPath")
	sessionJoinCol := aiSQLiteFirstPresent(sessionCols, "session_id", "sessionId", "id")
	if sessionJoinCol == "" {
		return nil, nil
	}

	query := "SELECT " + strings.Join([]string{
		"CAST(m." + aiSQLiteQuoteIdent(sessionCol) + " AS TEXT)",
		aiSQLiteNullableSelect("m", requestCol),
		"CAST(m." + aiSQLiteQuoteIdent(roleCol) + " AS TEXT)",
		aiSQLiteNullableSelect("m", toolResultCol),
		aiSQLiteNullableSelect("m", tokenInfoCol),
		aiSQLiteNullableSelect("m", contentCol),
		"CAST(m." + aiSQLiteQuoteIdent(timeCol) + " AS TEXT)",
		aiSQLiteNullableSelect("s", projectCol),
	}, ", ") + " FROM chat_message m LEFT JOIN chat_session s ON s." + aiSQLiteQuoteIdent(sessionJoinCol) + " = m." + aiSQLiteQuoteIdent(sessionCol) +
		" ORDER BY m." + aiSQLiteQuoteIdent(timeCol) + " ASC LIMIT 5000"

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query Qoder local DB: %w", err)
	}
	defer rows.Close()

	bySession := map[string]*aiTranscriptSummary{}
	qoderPromptLengths := qoderHistoryPromptLengths()
	qoderPromptOffsets := map[string]int{}
	for rows.Next() {
		var sessionID, requestID, role, toolResult, tokenInfo, content, rawTime, projectURI sql.NullString
		if err := rows.Scan(&sessionID, &requestID, &role, &toolResult, &tokenInfo, &content, &rawTime, &projectURI); err != nil {
			return nil, err
		}
		id := first(aiSQLString(sessionID), aiSQLString(requestID))
		if id == "" {
			continue
		}
		ts := parseQoderTime(aiSQLString(rawTime))
		if ts.IsZero() || ts.Before(after) {
			continue
		}
		summary := bySession[id]
		if summary == nil {
			summary = &aiTranscriptSummary{
				Source:      source.Name,
				SessionID:   id,
				CWD:         expandHome(trimFileURI(aiSQLString(projectURI))),
				Files:       map[string]bool{},
				FileWrites:  map[string]bool{},
				LineChanges: map[string]int{},
			}
			bySession[id] = summary
		}
		if ts.After(summary.LastActivity) {
			summary.LastActivity = ts
		}
		if summary.CWD == "" {
			summary.CWD = expandHome(trimFileURI(aiSQLString(projectURI)))
		}
		switch strings.ToLower(aiSQLString(role)) {
		case "assistant":
			if raw := aiSQLString(tokenInfo); raw != "" {
				updateAISummary(summary, map[string]any{"token_info": jsonObject(raw)})
			}
		case "tool":
			addQoderToolResult(summary, aiSQLString(toolResult))
		case "user":
			if text := aiSQLString(content); text != "" {
				summary.PromptLength += len(text)
			} else if length := nextQoderHistoryPromptLength(id, qoderPromptLengths, qoderPromptOffsets); length > 0 {
				summary.PromptLength += length
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]aiTranscriptSummary, 0, len(bySession))
	for _, summary := range bySession {
		if !summary.LastActivity.IsZero() {
			out = append(out, *summary)
		}
	}
	return out, nil
}

func qoderHistoryPromptLengths() map[string][]int {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	root := filepath.Join(home, ".qoder", "cache", "projects")
	paths, err := filepath.Glob(filepath.Join(root, "*", "conversation-history", "*", "*.jsonl"))
	if err != nil {
		return nil
	}
	lengthsByPrefix := map[string][]int{}
	for _, path := range paths {
		lengths := qoderPromptLengthsFromFile(path)
		if len(lengths) == 0 {
			continue
		}
		prefix := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		lengthsByPrefix[prefix] = append(lengthsByPrefix[prefix], lengths...)
	}
	return lengthsByPrefix
}

func qoderPromptLengthsFromFile(path string) []int {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil
	}
	defer file.Close()

	var lengths []int
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if !strings.EqualFold(firstString(line["role"]), "user") {
			continue
		}
		for _, text := range qoderHistoryTexts(line["message"]) {
			if prompt := qoderUserQuery(text); prompt != "" {
				lengths = append(lengths, len(prompt))
			}
		}
	}
	return lengths
}

func qoderHistoryTexts(value any) []string {
	var texts []string
	switch v := value.(type) {
	case map[string]any:
		if content, ok := v["content"].([]any); ok {
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok || firstString(block["type"]) != "text" {
					continue
				}
				if text := firstString(block["text"]); text != "" {
					texts = append(texts, text)
				}
			}
		}
	case []any:
		for _, item := range v {
			texts = append(texts, qoderHistoryTexts(item)...)
		}
	}
	return texts
}

func qoderUserQuery(text string) string {
	const (
		openTag  = "<user_query>"
		closeTag = "</user_query>"
	)
	text = strings.TrimSpace(text)
	start := strings.LastIndex(text, openTag)
	if start < 0 {
		return text
	}
	query := text[start+len(openTag):]
	if end := strings.Index(query, closeTag); end >= 0 {
		query = query[:end]
	}
	return strings.TrimSpace(query)
}

func nextQoderHistoryPromptLength(sessionID string, lengthsByPrefix map[string][]int, offsets map[string]int) int {
	var bestPrefix string
	var bestLengths []int
	for prefix, lengths := range lengthsByPrefix {
		if strings.HasPrefix(sessionID, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestLengths = lengths
		}
	}
	if bestPrefix == "" {
		return 0
	}
	offset := offsets[sessionID]
	if offset >= len(bestLengths) {
		return 0
	}
	offsets[sessionID] = offset + 1
	return bestLengths[offset]
}

func parseAIOpenCodeSQLite(db *sql.DB, source aiSQLiteSource, after time.Time) ([]aiTranscriptSummary, error) {
	if !aiSQLiteTableExists(db, "message") {
		return nil, nil
	}
	sessionInfo, err := openCodeSQLiteSessions(db)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
SELECT CAST(id AS TEXT), CAST(session_id AS TEXT), CAST(data AS TEXT), time_created
FROM message
WHERE json_valid(data)
ORDER BY time_created ASC, id ASC
LIMIT 5000`)
	if err != nil {
		return nil, fmt.Errorf("query OpenCode messages: %w", err)
	}
	defer rows.Close()

	bySession := map[string]*aiTranscriptSummary{}
	messageSessions := map[string]string{}
	for rows.Next() {
		var messageID, sessionID, data string
		var createdAt sql.NullInt64
		if err := rows.Scan(&messageID, &sessionID, &data, &createdAt); err != nil {
			return nil, err
		}
		if createdAt.Valid && time.UnixMilli(createdAt.Int64).Before(after) {
			continue
		}
		sessionID = first(sessionID, openCodeSQLiteSessionID(data), messageID)
		summary := openCodeSummaryForSession(bySession, sessionInfo, source.Name, sessionID)
		if createdAt.Valid {
			ts := time.UnixMilli(createdAt.Int64)
			if ts.After(summary.LastActivity) {
				summary.LastActivity = ts
			}
		}
		updateAISummary(summary, jsonObject(data))
		messageSessions[messageID] = sessionID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := addOpenCodeSQLiteParts(db, bySession, messageSessions); err != nil {
		return nil, err
	}
	out := make([]aiTranscriptSummary, 0, len(bySession))
	for _, summary := range bySession {
		if !summary.LastActivity.IsZero() && !summary.LastActivity.Before(after) {
			out = append(out, *summary)
		}
	}
	return out, nil
}

type openCodeSQLiteSession struct {
	Directory string
	Version   string
}

func openCodeSQLiteSessions(db *sql.DB) (map[string]openCodeSQLiteSession, error) {
	sessions := map[string]openCodeSQLiteSession{}
	if !aiSQLiteTableExists(db, "session") {
		return sessions, nil
	}
	rows, err := db.Query(`SELECT CAST(id AS TEXT), COALESCE(CAST(directory AS TEXT), ''), COALESCE(CAST(version AS TEXT), '') FROM session`)
	if err != nil {
		return nil, fmt.Errorf("query OpenCode sessions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, directory, version string
		if err := rows.Scan(&id, &directory, &version); err != nil {
			return nil, err
		}
		if strings.TrimSpace(id) != "" {
			sessions[id] = openCodeSQLiteSession{Directory: expandHome(directory), Version: version}
		}
	}
	return sessions, rows.Err()
}

func openCodeSummaryForSession(bySession map[string]*aiTranscriptSummary, sessionInfo map[string]openCodeSQLiteSession, source, sessionID string) *aiTranscriptSummary {
	if sessionID == "" {
		sessionID = "unknown"
	}
	summary := bySession[sessionID]
	if summary != nil {
		return summary
	}
	info := sessionInfo[sessionID]
	summary = &aiTranscriptSummary{
		AgentVersion: info.Version,
		CWD:          info.Directory,
		Files:        map[string]bool{},
		FileWrites:   map[string]bool{},
		LineChanges:  map[string]int{},
		SessionID:    sessionID,
		Source:       source,
	}
	bySession[sessionID] = summary
	return summary
}

func openCodeSQLiteSessionID(raw string) string {
	value := jsonObject(raw)
	if payload, ok := value.(map[string]any); ok {
		return firstString(payload["sessionID"], payload["session_id"])
	}
	return ""
}

func addOpenCodeSQLiteParts(db *sql.DB, bySession map[string]*aiTranscriptSummary, messageSessions map[string]string) error {
	if len(messageSessions) == 0 || !aiSQLiteTableExists(db, "part") {
		return nil
	}
	rows, err := db.Query(`
SELECT CAST(message_id AS TEXT), CAST(data AS TEXT), time_created
FROM part
WHERE json_valid(data)
ORDER BY time_created ASC, id ASC
LIMIT 5000`)
	if err != nil {
		return fmt.Errorf("query OpenCode parts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var messageID, data string
		var createdAt sql.NullInt64
		if err := rows.Scan(&messageID, &data, &createdAt); err != nil {
			return err
		}
		sessionID := messageSessions[messageID]
		if sessionID == "" {
			continue
		}
		summary := bySession[sessionID]
		if summary == nil {
			continue
		}
		if createdAt.Valid {
			ts := time.UnixMilli(createdAt.Int64)
			if ts.After(summary.LastActivity) {
				summary.LastActivity = ts
			}
		}
		updateAISummary(summary, jsonObject(data))
	}
	return rows.Err()
}

func addQoderToolResult(summary *aiTranscriptSummary, raw string) {
	if raw == "" {
		return
	}
	value := jsonObject(raw)
	updateAISummary(summary, value)
	if payload, ok := value.(map[string]any); ok {
		if path := normalizeAIPath(firstString(payload["projectPath"], payload["project_path"]), ""); path != "" && summary.CWD == "" {
			summary.CWD = filepath.Dir(path)
		}
	}
}

func aiSQLiteTableExists(db *sql.DB, table string) bool {
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=? LIMIT 1`, table).Scan(&name)
	return err == nil
}

func aiSQLiteTableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + aiSQLiteQuoteIdent(table) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var typ sql.NullString
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}

func aiSQLiteFirstPresent(cols map[string]bool, candidates ...string) string {
	for _, candidate := range candidates {
		if cols[strings.ToLower(candidate)] {
			return candidate
		}
	}
	return ""
}

func aiSQLiteSelectExpr(col string) string {
	if col == "" {
		return "NULL"
	}
	return aiSQLiteQuoteIdent(col)
}

func aiSQLiteNullableSelect(alias, col string) string {
	if col == "" {
		return "NULL"
	}
	return "CAST(" + alias + "." + aiSQLiteQuoteIdent(col) + " AS TEXT)"
}

func aiSQLiteQuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func aiSQLString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func aiSQLInt(value sql.NullInt64) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int64)
}

func sqliteSessionID(key string) string {
	key = strings.TrimPrefix(key, "bubbleId:")
	if i := strings.Index(key, ":"); i >= 0 {
		key = key[:i]
	}
	return strings.TrimSpace(key)
}

func parseQoderTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		if parsed > 1e10 {
			return time.UnixMilli(parsed)
		}
		return time.Unix(parsed, 0)
	}
	return parseAITimeString(value)
}

func trimFileURI(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "file://")
}

func jsonObject(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return map[string]any{}
	}
	return value
}

func firstString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
