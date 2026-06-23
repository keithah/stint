#!/usr/bin/env bash
# Cross-check Stint's Claude-only usage totals against ccusage.
#
# Stint prices per-token-type the same way ccusage does, so for Claude Code data
# the calculate-mode totals should match within rounding. Input and cache-read
# tokens are the strongest signals (output may differ by reasoning-token
# handling). A real divergence is a bug.
#
# Usage:
#   STINT_API_URL=http://localhost:8080/api/v1 \
#   STINT_API_KEY=waka_... \
#   scripts/ccusage-crosscheck.sh [range]
#
# range defaults to last_year (widest non-all-time window).
set -euo pipefail

range="${1:-last_year}"
api_url="${STINT_API_URL:-http://localhost:8080/api/v1}"
api_key="${STINT_API_KEY:?set STINT_API_KEY}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Running ccusage (calculate mode)..."
npx --yes ccusage@latest daily --mode calculate --json > "$tmp/ccusage.json"

echo "Fetching Stint summary (range=$range, mode=calculate, agent=claude)..."
# Scope to Claude so the comparison matches ccusage's Claude-only figures.
curl -fsS -H "Authorization: Bearer $api_key" \
  "$api_url/users/current/usage_events/summary?range=$range&cost_mode=calculate&agent=claude" > "$tmp/stint.json"

python3 - "$tmp/ccusage.json" "$tmp/stint.json" <<'PY'
import json, sys
cc = json.load(open(sys.argv[1]))
st = json.load(open(sys.argv[2]))["data"]["total"]

# ccusage is multi-agent; isolate Claude via per-day modelBreakdowns.
c_cost = c_in = c_out = c_cr = 0
for row in cc.get("daily", []):
    for m in row.get("modelBreakdowns", []) or []:
        if "claude" in (m.get("modelName") or m.get("model") or "").lower():
            c_cost += m.get("cost", 0) or 0
            c_in   += m.get("inputTokens", 0) or 0
            c_out  += m.get("outputTokens", 0) or 0
            c_cr   += m.get("cacheReadTokens", 0) or 0

def drift(a, b):
    return 0.0 if b == 0 else abs(a - b) / b * 100

rows = [
    ("cost USD",   st["cost_usd"],          c_cost),
    ("input",      st["input_tokens"],      c_in),
    ("output",     st["output_tokens"],     c_out),
    ("cache_read", st["cache_read_tokens"], c_cr),
]
print(f"{'metric':<12}{'stint':>18}{'ccusage':>18}{'drift %':>10}")
worst = 0.0
for name, s, c in rows:
    d = drift(s, c)
    worst = max(worst, d if name in ("input", "cache_read") else 0)
    print(f"{name:<12}{s:>18.2f}{c:>18.2f}{d:>9.2f}%")
# Gate on the high-confidence signals (input + cache-read).
print()
print("PASS" if worst < 1.0 else "FAIL", f"(input/cache-read drift {worst:.2f}%, threshold 1%)")
sys.exit(0 if worst < 1.0 else 1)
PY
