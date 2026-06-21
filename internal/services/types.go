package services

import (
	"encoding/json"
	"strings"
)

type Heartbeat struct {
	ID                 string         `json:"id,omitempty"`
	Entity             string         `json:"entity"`
	Type               string         `json:"type"`
	Category           string         `json:"category,omitempty"`
	Time               float64        `json:"time"`
	Project            string         `json:"project,omitempty"`
	Branch             string         `json:"branch,omitempty"`
	CommitHash         string         `json:"commit_hash,omitempty"`
	Revision           string         `json:"revision,omitempty"`
	Language           string         `json:"language,omitempty"`
	MachineName        string         `json:"machine_name,omitempty"`
	Plugin             string         `json:"plugin,omitempty"`
	PluginVersion      string         `json:"plugin_version,omitempty"`
	Editor             string         `json:"editor,omitempty"`
	EditorVersion      string         `json:"editor_version,omitempty"`
	OperatingSystem    string         `json:"operating_system,omitempty"`
	Architecture       string         `json:"architecture,omitempty"`
	Dependencies       string         `json:"dependencies,omitempty"`
	Lines              *int           `json:"lines,omitempty"`
	LineNumber         *int           `json:"lineno,omitempty"`
	CursorPosition     *int           `json:"cursorpos,omitempty"`
	IsWrite            bool           `json:"is_write,omitempty"`
	AILineChanges      *int           `json:"ai_line_changes,omitempty"`
	HumanLineChanges   *int           `json:"human_line_changes,omitempty"`
	AISession          string         `json:"ai_session,omitempty"`
	AIInputTokens      *int           `json:"ai_input_tokens,omitempty"`
	AIOutputTokens     *int           `json:"ai_output_tokens,omitempty"`
	AIPromptLength     *int           `json:"ai_prompt_length,omitempty"`
	AISubscriptionPlan string         `json:"ai_subscription_plan,omitempty"`
	AIModel            string         `json:"ai_model,omitempty"`
	AIProvider         string         `json:"ai_provider,omitempty"`
	AIAgent            string         `json:"ai_agent,omitempty"`
	AIAgentVersion     string         `json:"ai_agent_version,omitempty"`
	AIAgentComplexity  string         `json:"ai_agent_complexity,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	RawPayload         map[string]any `json:"raw_payload,omitempty"`
}

func (h *Heartbeat) UnmarshalJSON(data []byte) error {
	var rawPayload map[string]any
	if err := json.Unmarshal(data, &rawPayload); err != nil {
		return err
	}
	var raw struct {
		ID                 string          `json:"id,omitempty"`
		Entity             string          `json:"entity"`
		Type               string          `json:"type"`
		Category           string          `json:"category,omitempty"`
		Time               float64         `json:"time"`
		Project            string          `json:"project,omitempty"`
		AlternateProject   string          `json:"alternate_project,omitempty"`
		Branch             string          `json:"branch,omitempty"`
		CommitHash         string          `json:"commit_hash,omitempty"`
		Revision           string          `json:"revision,omitempty"`
		Language           string          `json:"language,omitempty"`
		MachineName        string          `json:"machine_name,omitempty"`
		MachineNameID      string          `json:"machine_name_id,omitempty"`
		Plugin             string          `json:"plugin,omitempty"`
		PluginVersion      string          `json:"plugin_version,omitempty"`
		Editor             string          `json:"editor,omitempty"`
		EditorVersion      string          `json:"editor_version,omitempty"`
		OperatingSystem    string          `json:"operating_system,omitempty"`
		Architecture       string          `json:"architecture,omitempty"`
		Dependencies       json.RawMessage `json:"dependencies,omitempty"`
		Lines              *int            `json:"lines,omitempty"`
		LineNumber         *int            `json:"lineno,omitempty"`
		CursorPosition     *int            `json:"cursorpos,omitempty"`
		IsWrite            bool            `json:"is_write,omitempty"`
		AILineChanges      *int            `json:"ai_line_changes,omitempty"`
		HumanLineChanges   *int            `json:"human_line_changes,omitempty"`
		AISession          string          `json:"ai_session,omitempty"`
		AIInputTokens      *int            `json:"ai_input_tokens,omitempty"`
		AIOutputTokens     *int            `json:"ai_output_tokens,omitempty"`
		AIPromptLength     *int            `json:"ai_prompt_length,omitempty"`
		AISubscriptionPlan string          `json:"ai_subscription_plan,omitempty"`
		AIModel            string          `json:"ai_model,omitempty"`
		AIModelName        string          `json:"ai_model_name,omitempty"`
		ModelName          string          `json:"model_name,omitempty"`
		LLMModel           string          `json:"llm_model,omitempty"`
		Model              string          `json:"model,omitempty"`
		AIProvider         string          `json:"ai_provider,omitempty"`
		Provider           string          `json:"provider,omitempty"`
		LLMProvider        string          `json:"llm_provider,omitempty"`
		AIAgent            string          `json:"ai_agent,omitempty"`
		AIAgentName        string          `json:"ai_agent_name,omitempty"`
		AIAgentVersion     string          `json:"ai_agent_version,omitempty"`
		AIAgentComplexity  string          `json:"ai_agent_complexity,omitempty"`
		Metadata           map[string]any  `json:"metadata,omitempty"`
		Meta               map[string]any  `json:"meta,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	dependencies, err := parseHeartbeatDependencies(raw.Dependencies)
	if err != nil {
		return err
	}
	*h = Heartbeat{
		ID:                 raw.ID,
		Entity:             raw.Entity,
		Type:               raw.Type,
		Category:           raw.Category,
		Time:               raw.Time,
		Project:            firstNonEmpty(raw.Project, raw.AlternateProject),
		Branch:             raw.Branch,
		CommitHash:         raw.CommitHash,
		Revision:           raw.Revision,
		Language:           raw.Language,
		MachineName:        firstNonEmpty(raw.MachineName, wakatimeMachineLabel(raw.MachineNameID)),
		Plugin:             raw.Plugin,
		PluginVersion:      raw.PluginVersion,
		Editor:             raw.Editor,
		EditorVersion:      raw.EditorVersion,
		OperatingSystem:    raw.OperatingSystem,
		Architecture:       raw.Architecture,
		Dependencies:       dependencies,
		Lines:              raw.Lines,
		LineNumber:         raw.LineNumber,
		CursorPosition:     raw.CursorPosition,
		IsWrite:            raw.IsWrite,
		AILineChanges:      raw.AILineChanges,
		HumanLineChanges:   raw.HumanLineChanges,
		AISession:          raw.AISession,
		AIInputTokens:      raw.AIInputTokens,
		AIOutputTokens:     raw.AIOutputTokens,
		AIPromptLength:     raw.AIPromptLength,
		AISubscriptionPlan: raw.AISubscriptionPlan,
		AIModel:            firstNonEmpty(raw.AIModel, raw.AIModelName, raw.ModelName, raw.LLMModel, raw.Model),
		AIProvider:         firstNonEmpty(raw.AIProvider, raw.Provider, raw.LLMProvider),
		AIAgent:            firstNonEmpty(raw.AIAgent, raw.AIAgentName),
		AIAgentVersion:     raw.AIAgentVersion,
		AIAgentComplexity:  raw.AIAgentComplexity,
		Metadata:           firstNonNilMap(raw.Metadata, raw.Meta),
		RawPayload:         rawPayload,
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNilMap(values ...map[string]any) map[string]any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func wakatimeMachineLabel(machineID string) string {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return ""
	}
	if len(machineID) > 8 {
		machineID = machineID[:8]
	}
	return "wakatime-" + machineID
}

func parseHeartbeatDependencies(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return "", err
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return strings.Join(normalized, ","), nil
}

type ExternalDuration struct {
	ID         string  `json:"id,omitempty"`
	Provider   string  `json:"provider"`
	ExternalID string  `json:"external_id"`
	Entity     string  `json:"entity"`
	Type       string  `json:"type"`
	Category   string  `json:"category,omitempty"`
	StartTime  float64 `json:"start_time"`
	EndTime    float64 `json:"end_time"`
	Project    string  `json:"project,omitempty"`
	Branch     string  `json:"branch,omitempty"`
	Language   string  `json:"language,omitempty"`
	Meta       string  `json:"meta,omitempty"`
}

type Duration struct {
	Name            string  `json:"name"`
	Project         string  `json:"project,omitempty"`
	Language        string  `json:"language,omitempty"`
	Time            float64 `json:"time"`
	DurationSeconds int     `json:"duration"`
}

type SliceTotal struct {
	Name         string `json:"name"`
	TotalSeconds int    `json:"total_seconds"`
	Text         string `json:"text"`
}

type CommitSummary struct {
	ID                            string  `json:"id"`
	Hash                          string  `json:"hash"`
	TruncatedHash                 string  `json:"truncated_hash"`
	Branch                        string  `json:"branch,omitempty"`
	Ref                           string  `json:"ref,omitempty"`
	Message                       string  `json:"message,omitempty"`
	TotalSeconds                  int     `json:"total_seconds"`
	HumanReadableTotal            string  `json:"human_readable_total"`
	HumanReadableTotalWithSeconds string  `json:"human_readable_total_with_seconds"`
	CreatedAt                     string  `json:"created_at,omitempty"`
	AuthorDate                    string  `json:"author_date,omitempty"`
	CommitterDate                 string  `json:"committer_date,omitempty"`
	HTMLURL                       string  `json:"html_url,omitempty"`
	URL                           string  `json:"url,omitempty"`
	LastHeartbeatAt               float64 `json:"-"`
}

type DailyStat struct {
	Date         string       `json:"date"`
	TotalSeconds int          `json:"total_seconds"`
	Text         string       `json:"text"`
	Projects     []SliceTotal `json:"projects"`
}

type WeekdayStat struct {
	Name           string `json:"name"`
	Day            int    `json:"day"`
	TotalSeconds   int    `json:"total_seconds"`
	Text           string `json:"text"`
	ActiveDays     int    `json:"active_days"`
	AverageSeconds int    `json:"average_seconds"`
	AverageText    string `json:"average_text"`
}

type DailyAverageTrendStat struct {
	Date           string `json:"date"`
	TotalSeconds   int    `json:"total_seconds"`
	Text           string `json:"text"`
	AverageSeconds int    `json:"average_seconds"`
	AverageText    string `json:"average_text"`
	DayCount       int    `json:"day_count"`
}

type HourlyStat struct {
	Hour         int          `json:"hour"`
	Label        string       `json:"label"`
	TotalSeconds int          `json:"total_seconds"`
	Text         string       `json:"text"`
	Projects     []SliceTotal `json:"projects"`
	Languages    []SliceTotal `json:"languages"`
}

type AIStat struct {
	Name               string `json:"name"`
	AISeconds          int    `json:"ai_seconds"`
	AILineChanges      int    `json:"ai_line_changes"`
	HumanLineChanges   int    `json:"human_line_changes"`
	AIInputTokens      int    `json:"ai_input_tokens"`
	AIOutputTokens     int    `json:"ai_output_tokens"`
	AIPromptLength     int    `json:"ai_prompt_length"`
	SessionCount       int    `json:"session_count"`
	EstimatedCostCents int    `json:"estimated_cost_cents"`
}

type AICostPeriod struct {
	Agent        string `json:"agent"`
	DailyCents   int    `json:"daily_cents"`
	WeeklyCents  int    `json:"weekly_cents"`
	MonthlyCents int    `json:"monthly_cents"`
	TotalCents   int    `json:"total_cents"`
}

type AIMetrics struct {
	AILineChanges         int            `json:"ai_line_changes"`
	HumanLineChanges      int            `json:"human_line_changes"`
	AIPercentage          int            `json:"ai_percentage"`
	HumanReviewPercentage int            `json:"human_review_percentage"`
	FollowUpEdits         int            `json:"follow_up_edits"`
	AIInputTokens         int            `json:"ai_input_tokens"`
	AIOutputTokens        int            `json:"ai_output_tokens"`
	AIPromptLength        int            `json:"ai_prompt_length"`
	PromptCount           int            `json:"prompt_count"`
	AveragePromptLength   int            `json:"average_prompt_length"`
	MedianPromptLength    int            `json:"median_prompt_length"`
	SessionCount          int            `json:"session_count"`
	EstimatedCostCents    int            `json:"estimated_cost_cents"`
	Agents                []AIStat       `json:"agents"`
	Days                  []AIStat       `json:"days"`
	Costs                 []AICostPeriod `json:"costs"`
}

type Stats struct {
	Range               string       `json:"range"`
	TotalSeconds        int          `json:"total_seconds"`
	HumanReadableTotal  string       `json:"human_readable_total"`
	DailyAverageSeconds int          `json:"daily_average_seconds"`
	HumanReadableDaily  string       `json:"human_readable_daily_average"`
	BestDay             DailyStat    `json:"best_day"`
	Days                []DailyStat  `json:"days"`
	Hourly              []HourlyStat `json:"hourly"`
	Projects            []SliceTotal `json:"projects"`
	Languages           []SliceTotal `json:"languages"`
	Editors             []SliceTotal `json:"editors"`
	OperatingSystems    []SliceTotal `json:"operating_systems"`
	Machines            []SliceTotal `json:"machines"`
	Categories          []SliceTotal `json:"categories"`
	Branches            []SliceTotal `json:"branches"`
	Dependencies        []SliceTotal `json:"dependencies"`
	AI                  AIMetrics    `json:"ai"`
	ProjectAI           []AIStat     `json:"project_ai"`
	IsUpToDate          bool         `json:"is_up_to_date"`
	PercentCalculated   int          `json:"percent_calculated"`
}

type StatusBarStats struct {
	TotalSeconds      int    `json:"total_seconds"`
	GrandTotalText    string `json:"grand_total_text"`
	Project           string `json:"project,omitempty"`
	ProjectSeconds    int    `json:"project_seconds,omitempty"`
	ProjectText       string `json:"project_text,omitempty"`
	Language          string `json:"language,omitempty"`
	LanguageSeconds   int    `json:"language_seconds,omitempty"`
	LanguageText      string `json:"language_text,omitempty"`
	Range             string `json:"range"`
	PercentCalculated int    `json:"percent_calculated"`
}

type Goal struct {
	ID               string   `json:"id,omitempty"`
	Title            string   `json:"title"`
	CustomTitle      string   `json:"custom_title,omitempty"`
	Delta            string   `json:"delta"`
	Seconds          int      `json:"seconds"`
	Languages        []string `json:"languages,omitempty"`
	Editors          []string `json:"editors,omitempty"`
	Projects         []string `json:"projects,omitempty"`
	IgnoreDays       []string `json:"ignore_days,omitempty"`
	IgnoreZeroDays   bool     `json:"ignore_zero_days"`
	ImproveByPercent *float64 `json:"improve_by_percent,omitempty"`
	IsEnabled        bool     `json:"is_enabled"`
	IsInverse        bool     `json:"is_inverse"`
	IsSnoozed        bool     `json:"is_snoozed"`
	SnoozeUntil      string   `json:"snooze_until,omitempty"`
	CreatedAt        string   `json:"created_at,omitempty"`
	ModifiedAt       string   `json:"modified_at,omitempty"`
}

type GoalProgress struct {
	Goal             Goal   `json:"goal"`
	ActualSeconds    int    `json:"actual_seconds"`
	TargetSeconds    int    `json:"target_seconds"`
	Percent          int    `json:"percent"`
	IsComplete       bool   `json:"is_complete"`
	HumanReadable    string `json:"human_readable_actual"`
	TargetReadable   string `json:"human_readable_target"`
	RemainingSeconds int    `json:"remaining_seconds"`
	IsSnoozed        bool   `json:"is_snoozed"`
	IsIgnored        bool   `json:"is_ignored"`
}

type LeaderboardEntry struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	Country      string `json:"country,omitempty"`
	TotalSeconds int    `json:"total_seconds"`
	Text         string `json:"text"`
	Rank         int    `json:"rank"`
}

type CustomRuleDestination struct {
	Destination      string `json:"destination"`
	DestinationValue string `json:"destination_value"`
}

type CustomRule struct {
	ID           string                  `json:"id,omitempty"`
	Action       string                  `json:"action"`
	Source       string                  `json:"source"`
	Operation    string                  `json:"operation"`
	SourceValue  string                  `json:"source_value"`
	Priority     int                     `json:"priority"`
	Destinations []CustomRuleDestination `json:"destinations,omitempty"`
	CreatedAt    string                  `json:"created_at,omitempty"`
	ModifiedAt   string                  `json:"modified_at,omitempty"`
}
