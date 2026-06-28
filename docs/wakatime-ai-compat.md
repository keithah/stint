# WakaTime AI Stats — Compatibility Spec

Goal: make Stint accept and expose the **same AI telemetry contract** that
`wakatime-cli` and the new first-party AI plugins (Claude Code, Codex CLI,
Antigravity, Amp, Cursor, …) use, so those plugins work against a self-hosted
Stint unchanged and any WakaTime-compatible client reads Stint's AI stats.

This tracks the AI features WakaTime announced in June 2026: AI Coding Insights
(lines you write vs. AI-generated), Model Usage & Cost Tracking, and Prompt
Insights (avg prompt length, avg/median prompts per session).

Sourced from the live WakaTime API docs (`wakatime.com/developers`) and the
`wakatime-cli` heartbeat package. Field definitions are quoted in
[Appendix A](#appendix-a--wakatime-ai-contract-verbatim).

---

## 1. Executive summary

Stint accepts every documented WakaTime AI heartbeat field and now mirrors the
new WakaTime read-side AI fields additively in stats and durations responses.
The remaining caveat is the additions/deletions split: WakaTime's heartbeat
wire field is a combined line-change count, so Stint exposes that combined
count as additions and keeps deletions at zero until a true split is available.

| Layer | Status |
|---|---|
| Heartbeat ingestion (`POST .../heartbeats.bulk`) | Implemented |
| `"ai coding"` category handling | Implemented |
| Stats response (`GET .../stats`) AI block | Implemented |
| Durations response (`GET .../durations`) AI fields | Implemented |
| Prompt Insights (per-session avg/median) | Implemented |

The implemented strategy is **additive mirroring**. Stint keeps its native
fields and emits WakaTime-named aliases alongside them so the existing dashboard
keeps working while WakaTime-compatible clients see the data they expect.

---

## 2. The WakaTime AI contract

### 2.1 Wire format — what plugins POST (heartbeat create)

`POST /api/v1/users/current/heartbeats.bulk`, per-heartbeat AI fields:

| Field | Type | Meaning |
|---|---|---|
| `category` | string | now includes `"ai coding"` |
| `ai_line_changes` | int | lines added **or** removed by GenAI since last heartbeat |
| `human_line_changes` | int | lines added or removed by typing since last heartbeat |
| `ai_session` | string | AI session id |
| `ai_input_tokens` | int | user input tokens since last heartbeat |
| `ai_output_tokens` | int | output tokens since last heartbeat |
| `ai_prompt_length` | int | user prompt chars since last heartbeat |
| `ai_subscription_plan` | string | GenAI tool subscription plan |

Note: the wire carries a **combined** line-change count, not a separate
additions/deletions split. The agent/model identity is inferred by WakaTime
from the plugin User-Agent; Stint additionally accepts explicit `ai_model` /
`ai_agent` fields.

### 2.2 Read format — what stats/durations return (derived server-side)

`GET …/stats` AI block:

```
ai_additions, ai_deletions, human_additions, human_deletions
ai_agent_line_changes   { "<agent>": <int lines> }
ai_line_changes_total   <int>
ai_agent_costs          { "<agent>": <float USD> }
ai_agent_breakdown      [ { name, lines, cost(USD) }, … ]
ai_agent_total_cost     <float USD>
ai_input_tokens, ai_output_tokens
ai_prompt_length_avg, ai_prompt_length_avg_per_session,
  ai_prompt_length_median_per_session, ai_prompt_length_sum
ai_prompt_events_total, ai_prompt_events_avg_per_session,
  ai_prompt_events_median_per_session
ai_sessions
```

`GET …/durations` exposes a per-duration subset of the same family
(`ai_additions/deletions`, `human_additions/deletions`, `ai_agent_costs`,
`ai_input_tokens`, `ai_output_tokens`, the `ai_prompt_*` metrics, `ai_sessions`).

---

## 3. What Stint already has

`internal/services/types.go` → `Heartbeat` accepts, via `UnmarshalJSON`, all of:
`ai_line_changes`, `human_line_changes`, `ai_session`, `ai_input_tokens`,
`ai_output_tokens`, `ai_prompt_length`, `ai_subscription_plan` — plus
`ai_model`/`model`/`llm_model`, `ai_provider`, `ai_agent`, `ai_agent_version`,
`ai_agent_complexity`, and `metadata`. So **plugin POSTs already round-trip.**

`internal/services/stats.go` aggregates these into `AIMetrics`:

| Stint field (`json`) | Notes |
|---|---|
| `ai_line_changes`, `human_line_changes` | combined counts |
| `ai_additions`, `ai_deletions`, `human_additions`, `human_deletions` | WakaTime aliases; combined counts are emitted as additions, deletions stay zero |
| `ai_line_changes_total`, `ai_agent_line_changes` | WakaTime line-change totals |
| `ai_percentage`, `human_review_percentage`, `follow_up_edits` | derived |
| `ai_input_tokens`, `ai_output_tokens`, `ai_prompt_length` | totals |
| `prompt_count`, `average_prompt_length`, `median_prompt_length` | native overall prompt metrics |
| `ai_prompt_length_avg`, `ai_prompt_length_sum`, `ai_prompt_length_avg_per_session`, `ai_prompt_length_median_per_session` | WakaTime prompt length metrics |
| `ai_prompt_events_total`, `ai_prompt_events_avg_per_session`, `ai_prompt_events_median_per_session` | WakaTime prompt event metrics |
| `session_count`, `ai_sessions` | native and WakaTime session counts |
| `estimated_cost_cents` | native integer cents |
| `ai_agent_costs`, `ai_agent_breakdown`, `ai_agent_total_cost` | WakaTime USD-float agent cost shape |
| `agents[]` (`AIStat`), `days[]`, `costs[]` (`AICostPeriod`) | per-agent / per-day / period spend |
| `project_ai[]` | per-project AI breakdown |

AI *time* (`ai_seconds`) is derived from durations whose heartbeats carry AI
fields or explicitly use `category == "ai coding"`.

---

## 4. Compatibility notes

**C1 — Response field-name mirroring.** Stint emits its native fields plus
WakaTime aliases in `AIMetrics`: `ai_additions`, `ai_deletions`,
`human_additions`, `human_deletions`, `ai_line_changes_total`,
`ai_agent_line_changes`, `ai_agent_costs`, `ai_agent_breakdown`,
`ai_agent_total_cost`, and `ai_sessions`.

**C2 — additions/deletions split.** Stint stores a single combined
`ai_line_changes`. The wire field is also combined, so a true add/delete split
isn't directly derivable. Stint currently exposes the combined count as
`ai_additions` / `human_additions` and returns zero deletions.

**C3 — Prompt Insights.** Stint groups prompt counts and prompt-length sums by
`ai_session`, then computes `ai_prompt_events_total`,
`ai_prompt_events_avg_per_session`, `ai_prompt_events_median_per_session`,
`ai_prompt_length_avg`, `ai_prompt_length_sum`,
`ai_prompt_length_avg_per_session`, and
`ai_prompt_length_median_per_session`.

**C4 — Durations AI fields.** `services.Duration` includes the WakaTime
per-duration AI family: line aliases, agent cost map, token totals, prompt
length/event metrics, and `ai_sessions`.

**C5 — `"ai coding"` category.** Stint treats explicit `category == "ai coding"`
as AI activity even when no other AI fields are present.

**C6 — Agent cost shape.** Stint keeps per-agent
`AIStat.estimated_cost_cents` and `AICostPeriod`, and also emits WakaTime's USD
float aliases: `ai_agent_costs`, `ai_agent_breakdown`, and
`ai_agent_total_cost`.

---

## 5. Verification coverage

The read-side compatibility behavior is covered by service and OpenAPI tests:

- `TestComputeStatsForRangeAggregatesAIMetrics`
- `TestComputeStatsForRangeCountsAICodingCategoryAsAISeconds`
- `TestComputeStatsForRangeWithAICostsUsesAgentRates`
- `TestComputeDurationsCarriesWakaTimeAIFields`
- OpenAPI schema tests for `AIMetrics` and `DurationRow`

The integration smoke path still exercises ingestion and WakaTime-compatible
client commands; keep extending it when new first-party plugin payloads appear.
Stint CLI AI sync coverage also pins source-specific transcript details such as
Codex user-prompt extraction from IDE/harness wrapper text, Claude
system-reminder cleanup, Claude subscription-plan metadata, successful direct
and shell `apply_patch` file heartbeats, and tool-call read/write metadata.

---

## 6. Open decisions

1. **additions vs. deletions (G2).** Options: (a) expose combined count as
   `ai_additions` with `ai_deletions: 0` and document it; (b) derive a split
   from successive `lines` (total-lines-in-file) deltas; (c) extend the wire
   with separate add/remove fields for Stint-aware plugins. Recommendation: (a)
   now for compatibility, revisit (b) if the dashboard needs a true split.
2. **Cost units.** Keep `estimated_cost_cents` (native) and add USD-float
   WakaTime fields rather than replacing them; this avoids breaking the current
   UI.

---

## Appendix A — WakaTime AI contract (verbatim)

Heartbeat create (`POST …/heartbeats.bulk`):

```
"category": … can be coding, …, designing, or ai coding
"ai_line_changes": <integer: lines added or removed by GenAI since last heartbeat in the current file (optional)>
"human_line_changes": <integer: lines added or removed by old-school typing since last heartbeat (optional)>
"ai_session": <string: AI session id (optional)>
"ai_input_tokens": <integer: user input tokens used since the last heartbeat by GenAI tools (optional)>
"ai_output_tokens": <integer: output tokens used since the last heartbeat by GenAI tools (optional)>
"ai_prompt_length": <integer: user prompt characters typed to AI since the last heartbeat (optional)>
"ai_subscription_plan": <string: subscription plan for the GenAI tool (optional)>
```

Stats (`GET …/stats`):

```
"ai_additions" / "ai_deletions" / "human_additions" / "human_deletions": <int>
"ai_agent_line_changes": { "<agent>": <int> }
"ai_line_changes_total": <int>
"ai_agent_costs": { "<agent>": <float USD> }
"ai_agent_breakdown": [ { "name", "lines": <int>, "cost": <float USD> }, … ]
"ai_agent_total_cost": <float USD>
"ai_input_tokens" / "ai_output_tokens": <int>
"ai_prompt_length_avg" / "ai_prompt_length_avg_per_session" /
  "ai_prompt_length_median_per_session" / "ai_prompt_length_sum": <int>
"ai_prompt_events_total" / "ai_prompt_events_avg_per_session" /
  "ai_prompt_events_median_per_session": <int>
"ai_sessions": <int>
```

Durations (`GET …/durations`): per-duration `ai_additions`, `ai_deletions`,
`human_additions`, `human_deletions`, `ai_agent_costs`, `ai_input_tokens`,
`ai_output_tokens`, the `ai_prompt_*` metrics, and `ai_sessions`.
