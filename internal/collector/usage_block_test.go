package collector

import "testing"

func TestAnthropicUsageCanonical(t *testing.T) {
	// Lumped cache-creation goes entirely to the 5m bucket; input is direct.
	u := anthropicUsageBlock{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 2000, CacheReadTokens: 10000}
	got := u.canonical()
	if got != (tokenUsage{Input: 100, Output: 50, CacheCreate5m: 2000, CacheRead: 10000}) {
		t.Fatalf("lumped: %+v", got)
	}
	// Explicit 5m/1h split wins over the lumped count.
	split := anthropicUsageBlock{InputTokens: 100, CacheCreationTokens: 2000}
	split.CacheCreation = &struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	}{Ephemeral5m: 1500, Ephemeral1h: 500}
	if got := split.canonical(); got.CacheCreate5m != 1500 || got.CacheCreate1h != 500 {
		t.Fatalf("split: %+v", got)
	}
}

func TestOpenAIUsageCanonicalSubtractsCached(t *testing.T) {
	// input_tokens is the total incl. cached; fresh input = input - cached.
	u := openAIUsageBlock{InputTokens: 5000, CachedInputTokens: 3000, OutputTokens: 200, ReasoningOutputTokens: 80}
	got := u.canonical()
	if got != (tokenUsage{Input: 2000, Output: 200, CacheRead: 3000, Reasoning: 80}) {
		t.Fatalf("openai: %+v", got)
	}
	// prompt_tokens / completion_tokens / details fallbacks.
	v := openAIUsageBlock{PromptTokens: 100, CompletionTokens: 40}
	v.PromptTokensDetails = &struct {
		CachedTokens int `json:"cached_tokens"`
	}{CachedTokens: 25}
	if got := v.canonical(); got.Input != 75 || got.Output != 40 || got.CacheRead != 25 {
		t.Fatalf("openai fallbacks: %+v", got)
	}
}

func TestGeminiUsageCanonical(t *testing.T) {
	u := geminiUsageBlock{PromptTokenCount: 6773, CandidatesTokenCount: 11, CachedContentTokenCount: 4925, ThoughtsTokenCount: 36}
	got := u.canonical()
	if got != (tokenUsage{Input: 6773 - 4925, Output: 11, CacheRead: 4925, Reasoning: 36}) {
		t.Fatalf("gemini: %+v", got)
	}
}
