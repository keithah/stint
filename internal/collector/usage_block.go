package collector

import (
	"encoding/json"

	"github.com/keithah/stint/internal/usage"
)

// tokenUsage is the canonical per-request token split every adapter targets.
// Centralizing it (and the two provider-shaped decoders below) keeps the
// conventions — Anthropic's cache-creation 5m/1h lump, OpenAI's input-minus-
// cached, and reasoning mapping — in one place instead of copy-pasted per agent.
type tokenUsage struct {
	Input         int
	Output        int
	CacheCreate5m int
	CacheCreate1h int
	CacheRead     int
	Reasoning     int
}

// apply writes the token counts onto an event. Adapters that read a non-JSON
// shape (SQLite columns, CSV, OTEL attributes) assemble a tokenUsage and call
// this so the six-field mapping lives once.
func (t tokenUsage) apply(e *usage.Event) {
	e.InputTokens = t.Input
	e.OutputTokens = t.Output
	e.CacheCreate5mTokens = t.CacheCreate5m
	e.CacheCreate1hTokens = t.CacheCreate1h
	e.CacheReadTokens = t.CacheRead
	e.ReasoningTokens = t.Reasoning
}

// anthropicUsageBlock is the Anthropic `message.usage` shape (Claude, Roo, Cline,
// Kilo, Kiro, OpenClaw, Amp, Factory Droid, …). input_tokens already excludes
// cache, so it maps through directly.
type anthropicUsageBlock struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreation       *struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// canonical applies the Anthropic conventions: prefer the explicit 5m/1h cache
// split, else lump cache_creation into the 5m bucket. Anthropic's output_tokens
// already includes thinking, so there is no separate reasoning field.
func (u anthropicUsageBlock) canonical() tokenUsage {
	t := tokenUsage{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadTokens}
	if u.CacheCreation != nil && (u.CacheCreation.Ephemeral5m != 0 || u.CacheCreation.Ephemeral1h != 0) {
		t.CacheCreate5m = u.CacheCreation.Ephemeral5m
		t.CacheCreate1h = u.CacheCreation.Ephemeral1h
	} else {
		t.CacheCreate5m = u.CacheCreationTokens
	}
	return t
}

// openAIUsageBlock is the OpenAI/Codex/Kimi `usage` shape. input/prompt tokens are
// the TOTAL including cached, so the fresh input is input-minus-cached and the
// cached portion is the cache read.
type openAIUsageBlock struct {
	InputTokens         int `json:"input_tokens"`
	PromptTokens        int `json:"prompt_tokens"`
	CachedInputTokens   int `json:"cached_input_tokens"`
	CachedTokens        int `json:"cached_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	OutputTokens          int `json:"output_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	OutputTokensDetails   *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (u openAIUsageBlock) canonical() tokenUsage {
	cached := firstNonZero(u.CachedInputTokens, u.CachedTokens)
	if cached == 0 && u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	reasoning := u.ReasoningOutputTokens
	if reasoning == 0 && u.OutputTokensDetails != nil {
		reasoning = u.OutputTokensDetails.ReasoningTokens
	}
	input := firstNonZero(u.InputTokens, u.PromptTokens) - cached
	if input < 0 {
		input = 0
	}
	return tokenUsage{
		Input:     input,
		Output:    firstNonZero(u.OutputTokens, u.CompletionTokens),
		CacheRead: cached,
		Reasoning: reasoning,
	}
}

// geminiUsageBlock is the Google GenAI usageMetadata shape (Gemini, Qwen). prompt
// tokens are the total including cached; thoughts are reasoning.
type geminiUsageBlock struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	PromptTokens            int `json:"prompt"`
	InputTokens             int `json:"input"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	OutputTokens            int `json:"output"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	CachedTokens            int `json:"cached"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	Thoughts                int `json:"thoughts"`
}

func (u geminiUsageBlock) canonical() tokenUsage {
	cached := firstNonZero(u.CachedContentTokenCount, u.CachedTokens)
	input := firstNonZero(u.PromptTokenCount, u.PromptTokens, u.InputTokens) - cached
	if input < 0 {
		input = 0
	}
	return tokenUsage{
		Input:     input,
		Output:    firstNonZero(u.CandidatesTokenCount, u.OutputTokens),
		CacheRead: cached,
		Reasoning: firstNonZero(u.ThoughtsTokenCount, u.Thoughts),
	}
}

// decodeAnthropicUsage / decodeOpenAIUsage decode a raw usage object for
// adapters that hold the block as a json.RawMessage.
func decodeAnthropicUsage(raw json.RawMessage) (tokenUsage, bool) {
	var u anthropicUsageBlock
	if err := json.Unmarshal(raw, &u); err != nil {
		return tokenUsage{}, false
	}
	return u.canonical(), true
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
