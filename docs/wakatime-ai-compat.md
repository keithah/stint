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

Stint already does the hard part. **Ingestion is fully compatible**: the
heartbeat handler accepts every documented WakaTime AI field (and several
extras). The gap is on the **read side** — Stint's stats/durations JSON uses
its own field names and is missing WakaTime's new per-session prompt metrics
and additions/deletions split.

| Layer | Status |
|---|---|
| Heartbeat ingestion (`POST …/heartbeats.bulk`) | ✅ Compatible today |
| `"ai coding"` category handling | ⚠️ Accepted but not surfaced as AI time |
| Stats response (`GET …/stats`) AI block | ⚠️ Different field names; missing metrics |
| Durations response (`GET …/durations`) AI fields | ❌ Not exposed at all |
| Prompt Insights (per-session avg/median) | ❌ Not computed |

Recommended strategy: **additive mirroring.** Keep Stint's native fields,
and emit WakaTime-named aliases alongside them so the existing dashboard keeps
working while WakaTime-compatible clients see the data they expect. Nothing
below is a breaking change.

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
| `ai_percentage`, `human_review_percentage`, `follow_up_edits` | derived |
| `ai_input_tokens`, `ai_output_tokens`, `ai_prompt_length` | totals |
| `prompt_count`, `average_prompt_length`, `median_prompt_length` | overall, not per-session |
| `session_count` | = WakaTime `ai_sessions` |
| `estimated_cost_cents` | integer **cents**, not USD float |
| `agents[]` (`AIStat`), `days[]`, `costs[]` (`AICostPeriod`) | per-agent / per-day / period spend |
| `project_ai[]` | per-project AI breakdown |

AI *time* (`ai_seconds`) is derived from durations whose heartbeats carry AI
fields — **not** from `category == "ai coding"`.

---

## 4. Gap analysis

**G1 — Response field-name mismatch.** Stint emits `ai_line_changes` /
`estimated_cost_cents`; WakaTime clients look for `ai_additions`/`ai_deletions`
and `ai_agent_costs`/`ai_agent_total_cost` in **USD floats**. A WakaTime-shaped
reader finds nothing.

**G2 — additions/deletions split.** Stint stores a single combined
`ai_line_changes`. The wire field is also combined, so a true add/delete split
isn't directly derivable. Decision needed (§6).

**G3 — Prompt Insights (the headline new feature) missing.** No
`ai_prompt_events_total`, `…_avg_per_session`, `…_median_per_session`, nor
`ai_prompt_length_avg_per_session` / `…_median_per_session` / `…_sum`. Stint has
overall length avg/median + a prompt count + session count, but nothing grouped
**per session**. (Stint's own `docs/SPEC.md` §8 already lists "median prompts
per session" as intended — this closes that gap too.)

**G4 — Durations carry no AI fields.** `services.Duration` is
`{name, project, language, time, duration}`. WakaTime's durations now include
the AI family.

**G5 — `"ai coding"` category not first-class.** Stored but not counted in the
`categories` slice as AI time, and AI-seconds ignore it. WakaTime treats it as a
real category.

**G6 — Agent cost shape.** Stint has per-agent `AIStat.estimated_cost_cents`
and `AICostPeriod`; WakaTime wants `ai_agent_costs` map + `ai_agent_breakdown`
array + `ai_agent_total_cost`, all USD floats, plus `ai_agent_line_changes` /
`ai_line_changes_total`.

---

## 5. Implementation plan (phased, additive)

**Phase 0 — Lock ingestion parity (tests only).** Add a round-trip test (and a
case in `scripts/smoke-wakatime.sh`) sending a heartbeat with every WakaTime AI
field incl. `category:"ai coding"`, asserting all persist and feed stats. Low
risk; proves the claim in §3.

**Phase 1 — `"ai coding"` as a real category (G5).** Count `category=="ai
coding"` heartbeats toward the `categories` breakdown and toward AI seconds
(union with the existing "has AI fields" rule). Keeps Stint's heuristic while
honoring the explicit category.

**Phase 2 — WakaTime-named stats aliases (G1, G6).** Extend the `AIMetrics`
JSON (additively) with: `ai_additions`/`ai_deletions`, `human_additions`/
`human_deletions`, `ai_line_changes_total`, `ai_agent_line_changes`,
`ai_agent_costs` (USD), `ai_agent_breakdown`, `ai_agent_total_cost` (USD),
`ai_sessions`. Cents→USD is `cents/100.0`.

**Phase 3 — Prompt Insights (G3).** During aggregation, group prompt counts and
prompt-length sums by `ai_session`, then compute `ai_prompt_events_total`,
`…_avg_per_session`, `…_median_per_session`, `ai_prompt_length_avg`,
`…_avg_per_session`, `…_median_per_session`, `…_sum`. Surface on the AI dashboard
panel (`web/components/ai-panel.tsx`).

**Phase 4 — Durations AI fields (G4).** Add the per-duration AI family to
`services.Duration` and the durations computation, mirroring the stats names.

**Phase 5 — Verify.** Unit tests for the per-session math (incl. empty/single-
session edge cases); extend `smoke-wakatime.sh`; re-run `ccusage-crosscheck.sh`
to confirm cost totals are unchanged.

Suggested order: Phase 0 → 2 → 3 (highest user value: the new dashboard
metrics) → 1 → 4.

---

## 6. Decisions (resolved)

See the consolidated decision log in
[`integrations-scope.md`](./integrations-scope.md) §8. Summary:

1. **additions vs. deletions (G2) — A1.** The wire sends a combined count, so
   expose it as `ai_additions` with `ai_deletions: 0`, documented. Revisit
   deriving a true split from successive `lines` (total-lines-in-file) deltas
   later if the dashboard needs it.
2. **Cost units — A2.** Keep `estimated_cost_cents` (native) and **add**
   USD-float WakaTime fields (`cents/100`), rather than replacing — no UI break.
3. **Per-session prompt counts — A3.** Confirmed: `PromptCount` is already
   `len(promptLengths)` — one prompt per heartbeat carrying `ai_prompt_length`.
   Group that signal and prompt-length by `ai_session` for the avg/median-per-
   session metrics.

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
