#!/usr/bin/env bash
set -Eeuo pipefail
trap 'echo "Smoke failed near line ${LINENO}: ${BASH_COMMAND}" >&2' ERR

api_base="${API_BASE:-http://localhost:8080}"
local_compose=0
if [[ "${api_base}" == "http://localhost:8080" ]]; then
  local_compose=1
fi
cookie_jar="$(mktemp)"
headers_file="$(mktemp)"
cli_home=""
cli_project_dir=""
import_tmp=""
trap 'rm -f "${cookie_jar}" "${headers_file}"; if [[ -n "${cli_home:-}" ]]; then rm -rf "${cli_home}"; fi; if [[ -n "${cli_project_dir:-}" ]]; then rm -rf "${cli_project_dir}"; fi; if [[ -n "${import_tmp:-}" ]]; then rm -rf "${import_tmp}"; fi' EXIT

current_stats_until_contains() {
  local needle="$1"
  local bearer_key="${2:-${api_key}}"
  local stats=""
  for _ in $(seq 1 40); do
    stats="$(curl -fsS \
      -H "Authorization: Bearer ${bearer_key}" \
      "${api_base}/api/v1/users/current/stats/last_7_days" || true)"
    if grep -q -- "${needle}" <<<"${stats}"; then
      printf '%s' "${stats}"
      return 0
    fi
    sleep 0.25
  done
  echo "Stats did not refresh with ${needle}" >&2
  return 1
}

assert_json_array() {
  local label="$1"
  node -e '
let body = "";
const label = process.argv[1];
process.stdin.on("data", (chunk) => { body += chunk; });
process.stdin.on("end", () => {
  const parsed = JSON.parse(body);
  if (!Array.isArray(parsed)) {
    console.error(`${label} was not a top-level JSON array`);
    process.exit(1);
  }
});
' "${label}"
}

api_ready=0
for _ in $(seq 1 40); do
  if curl -fsS "${api_base}/healthz" >/dev/null 2>&1; then
    api_ready=1
    break
  fi
  sleep 0.25
done
if [[ "${api_ready}" != "1" ]]; then
  echo "API did not become ready at ${api_base}" >&2
  exit 1
fi

if [[ "${local_compose}" == "1" ]]; then
  compose_web_base_count="$(docker compose config | grep -c 'WEB_BASE_URL: http://localhost:3001' || true)"
  if [[ "${compose_web_base_count}" -lt 2 ]]; then
    echo "Docker Compose WEB_BASE_URL must point API and worker redirects at http://localhost:3001" >&2
    exit 1
  fi
  unauth_oauth_location="$(curl -sS -o /dev/null -w "%{redirect_url}" "${api_base}/oauth/authorize")"
  if [[ "${unauth_oauth_location}" != "http://localhost:3001/login" ]]; then
    echo "Unauthenticated OAuth browser flow redirected to ${unauth_oauth_location}, expected http://localhost:3001/login" >&2
    exit 1
  fi
fi

curl -fsS "${api_base}/api/v1/editors" | grep -q '"key":"cursor"'
program_languages_json="$(curl -fsS "${api_base}/api/v1/program_languages")"
printf '%s' "${program_languages_json}" | grep -q '"name":"Ruby"'
printf '%s' "${program_languages_json}" | grep -q '"color":"#701516"'
curl -fsS "${api_base}/api/v1/leaders" >/dev/null
curl -fsS -H "X-Forwarded-For: 203.0.113.42" "${api_base}/api/v1/meta" | grep -q '"ip":"203.0.113.42"'
docs_json="$(curl -fsS "${api_base}/api/v1/docs")"
printf '%s' "${docs_json}" | grep -q '"/api/v1/users/current/heartbeats"'
printf '%s' "${docs_json}" | grep -q '"schemas"'
printf '%s' "${docs_json}" | grep -q '"BearerAuth"'
printf '%s' "${docs_json}" | grep -q '"BasicAuth"'
printf '%s' "${docs_json}" | grep -q '"OAuthClientBasic"'
printf '%s' "${docs_json}" | grep -q '"ApiKeyQuery"'
printf '%s' "${docs_json}" | grep -q '"name":"api_key"'
printf '%s' "${docs_json}" | node -e '
let body = "";
process.stdin.on("data", (chunk) => { body += chunk; });
process.stdin.on("end", () => {
  const security = JSON.parse(body).paths["/oauth/token"].post.security || [];
  if (!security.some((entry) => Object.prototype.hasOwnProperty.call(entry, "OAuthClientBasic"))) {
    process.exit(1);
  }
});
'
printf '%s' "${docs_json}" | grep -q '"Heartbeat"'
printf '%s' "${docs_json}" | grep -q '"alternate_project"'
printf '%s' "${docs_json}" | grep -q '"dependencies"'
printf '%s' "${docs_json}" | grep -q '"lineno"'
printf '%s' "${docs_json}" | grep -q '"cursorpos"'
printf '%s' "${docs_json}" | grep -q '"plugin_version"'
printf '%s' "${docs_json}" | grep -q '"architecture"'
printf '%s' "${docs_json}" | grep -q '"requestBody"'
printf '%s' "${docs_json}" | node -e '
let body = "";
process.stdin.on("data", (chunk) => { body += chunk; });
process.stdin.on("end", () => {
  const schema = JSON.parse(body).components.schemas.StatsRangesResponse.properties.data;
  if (schema.type !== "object" || schema.additionalProperties?.$ref !== "#/components/schemas/Stats") {
    process.exit(1);
  }
});
'
printf '%s' "${docs_json}" | node -e '
let body = "";
process.stdin.on("data", (chunk) => { body += chunk; });
process.stdin.on("end", () => {
  const params = JSON.parse(body).paths["/api/v1/users/current/insights/{insight_type}/{range}"].get.parameters || [];
  const insightType = params.find((param) => param.name === "insight_type");
  const values = insightType?.schema?.enum || [];
  for (const expected of ["dependencies", "hours", "daily_average_trend", "ai_agents"]) {
    if (!values.includes(expected)) {
      process.exit(1);
    }
  }
});
'
printf '%s' "${docs_json}" | node -e '
let body = "";
process.stdin.on("data", (chunk) => { body += chunk; });
process.stdin.on("end", () => {
  const schemas = JSON.parse(body).components.schemas;
  const values = schemas.DataDumpRequest.properties.type.enum || [];
  const goalDelta = schemas.GoalInput.properties.delta.enum || [];
  const goalSecondsMin = schemas.GoalInput.properties.seconds.minimum;
  const goalImproveMin = schemas.GoalInput.properties.improve_by_percent.minimum;
  const leaderboardPattern = schemas.LeaderboardInput.properties.time_range.pattern;
  const userUpdate = schemas.UserUpdateRequest.properties;
  const redirectURIs = schemas.OAuthAppCreateRequest.properties.redirect_uris;
  const oauthAppCreate = schemas.OAuthAppCreateRequest.properties;
  const leaderboardInput = schemas.LeaderboardInput.properties;
  const apiKeyCreate = schemas.APIKeyCreateRequest.properties;
  const shareCreate = schemas.ShareTokenCreateRequest.properties;
  const externalDuration = schemas.ExternalDuration.properties;
  const customRule = schemas.CustomRule.properties;
  const aiCost = schemas.AICostSetting.properties;
  const expectedLeaderboardPattern = "^(last_7_days|last_30_days|last_6_months|last_year|all_time|[0-9]{4}|[0-9]{4}-[0-9]{2})$";
  if (
    values.join(",") !== "heartbeats,daily" ||
    goalDelta.join(",") !== "day,week" ||
    customRule.action.enum.join(",") !== "change,delete" ||
    customRule.operation.enum.join(",") !== "equals,contains,starts_with,ends_with,regex,matches" ||
    goalSecondsMin !== 0 ||
    goalImproveMin !== 0 ||
    leaderboardPattern !== expectedLeaderboardPattern ||
    userUpdate.country.pattern !== "^[A-Za-z]{2}$" ||
    userUpdate.timezone.pattern !== "^(UTC|[A-Za-z_]+(/[A-Za-z0-9_+\\-]+)+)$" ||
    userUpdate.timeout_minutes.minimum !== 0 ||
    userUpdate.heartbeat_retention_days.minimum !== 0 ||
    redirectURIs.minItems !== 1 ||
    redirectURIs.items?.format !== "uri" ||
    oauthAppCreate.name.minLength !== 1 ||
    leaderboardInput.name.minLength !== 1 ||
    apiKeyCreate.name.minLength !== 1 ||
    shareCreate.name.minLength !== 1 ||
    externalDuration.start_time.exclusiveMinimum !== 0 ||
    externalDuration.end_time.exclusiveMinimum !== 0 ||
    aiCost.input_cost_per_million_cents.minimum !== 0 ||
    aiCost.output_cost_per_million_cents.minimum !== 0
  ) {
    process.exit(1);
  }
});
'

seed_json="$(curl -fsS -c "${cookie_jar}" -X POST "${api_base}/api/v1/dev/seed-key")"
api_key="$(printf '%s' "${seed_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
access_token="$(printf '%s' "${seed_json}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
user_id="$(printf '%s' "${seed_json}" | sed -n 's/.*"user":{"id":"\([^"]*\)".*/\1/p')"

if [[ -z "${api_key}" || -z "${access_token}" || -z "${user_id}" ]]; then
  echo "Could not read credentials from dev seed response" >&2
  exit 1
fi

now="$(date +%s)"
smoke_commit_hash="abcdef1234567890${now}"

curl -fsS \
  -H "Authorization: Bearer ${access_token}" \
  "${api_base}/api/v1/users/current" | grep -q '"github_username":"local-dev"'

curl -fsS \
  -u "${api_key}:" \
  "${api_base}/api/v1/users/current" | grep -q '"github_username":"local-dev"'

curl -fsS \
  "${api_base}/api/v1/users/current?api_key=${api_key}" | grep -q '"github_username":"local-dev"'

invalid_key_status="$(curl -sS -o /tmp/stint-invalid-api-key-response.json -w "%{http_code}" \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"   ","scopes":["read_stats"]}' \
  "${api_base}/api/v1/api_keys")"
if [[ "${invalid_key_status}" != "400" ]]; then
  echo "Expected invalid API key name to return 400, got ${invalid_key_status}" >&2
  cat /tmp/stint-invalid-api-key-response.json >&2
  exit 1
fi
grep -q 'API key name is required' /tmp/stint-invalid-api-key-response.json

scoped_key_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Read-only smoke","scopes":["read_stats"]}' \
  "${api_base}/api/v1/api_keys")"
scoped_key="$(printf '%s' "${scoped_key_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
printf '%s' "${scoped_key_json}" | grep -q '"scopes":\["read_stats"\]'

summary_language_key_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Language summaries smoke","scopes":["read_summaries.languages"]}' \
  "${api_base}/api/v1/api_keys")"
summary_language_key="$(printf '%s' "${summary_language_key_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
printf '%s' "${summary_language_key_json}" | grep -q '"scopes":\["read_summaries.languages"\]'

summary_category_key_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Category summaries smoke","scopes":["read_summaries.categories"]}' \
  "${api_base}/api/v1/api_keys")"
summary_category_key="$(printf '%s' "${summary_category_key_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
printf '%s' "${summary_category_key_json}" | grep -q '"scopes":\["read_summaries.categories"\]'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current" | grep -q '"email":"local-dev@example.com"'

scoped_profile_json="$(curl -fsS \
  -H "Authorization: Bearer ${scoped_key}" \
  "${api_base}/api/v1/users/current")"
if printf '%s' "${scoped_profile_json}" | grep -q '"email"'; then
  echo "Scoped API key without email scope unexpectedly received email" >&2
  exit 1
fi

scoped_key_create_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${scoped_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Should not create"}' \
  "${api_base}/api/v1/api_keys")"
if [[ "${scoped_key_create_status}" != "403" ]]; then
  echo "Scoped API key unexpectedly managed API keys" >&2
  exit 1
fi

scoped_profile_update_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -X PUT \
  -H "Authorization: Bearer ${scoped_key}" \
  -H "Content-Type: application/json" \
  -d '{"timezone":"UTC","timeout_minutes":15,"writes_only":false,"has_public_profile":false,"country":"US","heartbeat_retention_days":0}' \
  "${api_base}/api/v1/users/current")"
if [[ "${scoped_profile_update_status}" != "403" ]]; then
  echo "Scoped API key unexpectedly updated profile settings" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${scoped_key}" \
  "${api_base}/api/v1/users/current/stats/last_7_days" >/dev/null

scoped_write_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${scoped_key}" \
  -H "Content-Type: application/json" \
  -d "{\"entity\":\"/tmp/scoped-denied-${now}.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":${now},\"project\":\"denied\",\"language\":\"Go\"}" \
  "${api_base}/api/v1/users/current/heartbeats")"
if [[ "${scoped_write_status}" != "403" ]]; then
  echo "Scoped read-only API key unexpectedly wrote heartbeat" >&2
  exit 1
fi

invalid_settings_status="$(curl -sS -o /tmp/stint-invalid-settings-response.json -w "%{http_code}" \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"timezone":"Bad/Zone","timeout_minutes":15,"writes_only":false,"has_public_profile":false,"country":"US","heartbeat_retention_days":0}' \
  "${api_base}/api/v1/users/current")"
if [[ "${invalid_settings_status}" != "400" ]]; then
  echo "Expected invalid user settings to return 400, got ${invalid_settings_status}" >&2
  cat /tmp/stint-invalid-settings-response.json >&2
  exit 1
fi
grep -q '"timezone must be a valid IANA timezone' /tmp/stint-invalid-settings-response.json

zero_timeout_profile="$(curl -fsS \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"timezone":"UTC","timeout_minutes":0,"writes_only":false,"has_public_profile":false,"country":"US","heartbeat_retention_days":0}' \
  "${api_base}/api/v1/users/current")"
printf '%s' "${zero_timeout_profile}" | grep -q '"timeout_minutes":0'

curl -fsS \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"timezone":"UTC","timeout_minutes":15,"writes_only":false,"has_public_profile":false,"country":"US","heartbeat_retention_days":0}' \
  "${api_base}/api/v1/users/current" >/dev/null

private_profile_status="$(curl -sS -o /dev/null -w "%{http_code}" "${api_base}/api/v1/users/local-dev")"
if [[ "${private_profile_status}" != "404" ]]; then
  echo "Private profile was unexpectedly public" >&2
  exit 1
fi

curl -fsS \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"timezone":"UTC","timeout_minutes":15,"writes_only":false,"has_public_profile":true,"country":"US","heartbeat_retention_days":0}' \
  "${api_base}/api/v1/users/current" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -H "User-Agent: wakatime/v1.102.1 (linux-amd64) go1.23.0 vscode/1.89.0 vscode-wakatime/24.3.0" \
  -d "[{\"entity\":\"/tmp/stint-smoke/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":${now},\"project\":\"stint\",\"branch\":\"main\",\"language\":\"Go\",\"dependencies\":[\"pgx\"],\"machine_name\":\"local\",\"revision\":\"${smoke_commit_hash}\",\"ai_line_changes\":40,\"human_line_changes\":20,\"ai_session\":\"smoke-session\",\"ai_input_tokens\":1200,\"ai_output_tokens\":400,\"ai_prompt_length\":180,\"ai_subscription_plan\":\"Codex\"},{\"entity\":\"/tmp/stint-smoke/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((now + 300)),\"project\":\"stint\",\"branch\":\"main\",\"language\":\"Go\",\"dependencies\":\"pgx\",\"machine_name\":\"local\",\"commit_hash\":\"${smoke_commit_hash}\",\"ai_line_changes\":10,\"human_line_changes\":20,\"ai_session\":\"smoke-session\",\"ai_input_tokens\":600,\"ai_output_tokens\":200,\"ai_prompt_length\":120,\"ai_subscription_plan\":\"Codex\"}]" \
  "${api_base}/api/v1/users/current/heartbeats.bulk" >/dev/null

user_agent_heartbeats_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/heartbeats?date=$(date -u -d "@${now}" +%F)")"
printf '%s' "${user_agent_heartbeats_json}" | grep -q '"plugin":"wakatime"'
printf '%s' "${user_agent_heartbeats_json}" | grep -q '"plugin_version":"v1.102.1"'
printf '%s' "${user_agent_heartbeats_json}" | grep -q '"editor_version":"1.89.0"'
printf '%s' "${user_agent_heartbeats_json}" | grep -q '"architecture":"amd64"'

curl -fsS \
  -H "Authorization: Bearer ${summary_language_key}" \
  "${api_base}/api/v1/users/current/durations?date=$(date -u +%F)&slice_by=language" | grep -q '"language":"Go"'

summary_project_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${summary_language_key}" \
  "${api_base}/api/v1/users/current/durations?date=$(date -u +%F)&slice_by=project")"
if [[ "${summary_project_status}" != "403" ]]; then
  echo "Language-only summary scope unexpectedly read project durations" >&2
  exit 1
fi

summary_language_json="$(curl -fsS \
  -H "Authorization: Bearer ${summary_language_key}" \
  "${api_base}/api/v1/users/current/summaries?start=$(date -u +%F)&end=$(date -u +%F)")"
printf '%s' "${summary_language_json}" | grep -q '"languages"'
if printf '%s' "${summary_language_json}" | grep -q '"projects"'; then
  echo "Language-only summary scope unexpectedly received project summaries" >&2
  exit 1
fi

summary_category_json="$(curl -fsS \
  -H "Authorization: Bearer ${summary_category_key}" \
  "${api_base}/api/v1/users/current/summaries?start=$(date -u +%F)&end=$(date -u +%F)")"
printf '%s' "${summary_category_json}" | grep -q '"categories"'
if printf '%s' "${summary_category_json}" | grep -q '"projects"\|"languages"'; then
  echo "Category-only summary scope unexpectedly received project or language summaries" >&2
  exit 1
fi

wakatime_cli_bin="${WAKATIME_CLI_BIN:-}"
if [[ -z "${wakatime_cli_bin}" ]] && command -v wakatime-cli >/dev/null 2>&1; then
  wakatime_cli_bin="$(command -v wakatime-cli)"
fi
if [[ -z "${wakatime_cli_bin}" && -x ".tmp/bin/wakatime-cli" ]]; then
  wakatime_cli_bin=".tmp/bin/wakatime-cli"
fi
if [[ -n "${wakatime_cli_bin}" ]]; then
  cli_home="$(mktemp -d)"
  cli_project_dir="$(mktemp -d)"
  cli_project="cli-smoke-${now}"
  printf '%s\n' "${cli_project}" > "${cli_project_dir}/.wakatime-project"
  cli_entity="${cli_project_dir}/main.go"
  printf 'package main\n' > "${cli_entity}"
  printf '[settings]\ndebug = true\napi_url = %s/api/v1\napi_key = %s\noffline = false\ntimeout = 10\n' "${api_base}" "${api_key}" > "${cli_home}/.wakatime.cfg"
  WAKATIME_HOME="${cli_home}" "${wakatime_cli_bin}" \
    --entity "${cli_entity}" \
    --plugin "stint-smoke/1.0.0" \
    --time "${now}.0" \
    --write \
    --alternate-project "${cli_project}" \
    --config "${cli_home}/.wakatime.cfg" \
    --verbose
  cli_inserted=0
  for _ in $(seq 1 20); do
    if curl -fsS \
      -H "Authorization: Bearer ${api_key}" \
      "${api_base}/api/v1/users/current/projects/${cli_project}" >/dev/null 2>&1; then
      cli_inserted=1
      break
    fi
    sleep 0.25
  done
  if [[ "${cli_inserted}" != "1" ]]; then
    echo "wakatime-cli heartbeat did not create ${cli_project}" >&2
    exit 1
  fi
  cli_today_output="$(WAKATIME_HOME="${cli_home}" "${wakatime_cli_bin}" \
    --today \
    --config "${cli_home}/.wakatime.cfg" \
    --verbose)"
  if [[ -z "${cli_today_output}" ]]; then
    echo "wakatime-cli --today returned empty output" >&2
    exit 1
  fi
  cli_file_experts_output="$(WAKATIME_HOME="${cli_home}" "${wakatime_cli_bin}" \
    --entity "${cli_entity}" \
    --file-experts \
    --alternate-project "${cli_project}" \
    --config "${cli_home}/.wakatime.cfg" \
    --verbose)"
  if [[ -z "${cli_file_experts_output}" ]]; then
    echo "wakatime-cli --file-experts returned empty output" >&2
    exit 1
  fi
fi

if [[ "${local_compose}" == "1" ]]; then
  old_time=$((now - (400 * 86400)))
  old_date="$(date -u -d "@${old_time}" +%F)"
  old_entity="/tmp/purge-smoke-${now}/old.go"
  docker compose exec -T postgres psql -U stint -d stint \
    -c "insert into heartbeats (user_id, entity, type, category, time, project, language, machine_name) values ('${user_id}', '${old_entity}', 'file', 'coding', ${old_time}, 'purge-smoke', 'Go', 'local') on conflict do nothing;" >/dev/null

  curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/heartbeats?date=${old_date}" | grep -q "purge-smoke-${now}"

  curl -fsS -X POST "${api_base}/api/v1/dev/jobs/heartbeats-purge?retention_days=365" >/dev/null

  purged=0
  for _ in $(seq 1 20); do
    old_heartbeats_json="$(curl -fsS \
      -H "Authorization: Bearer ${api_key}" \
      "${api_base}/api/v1/users/current/heartbeats?date=${old_date}")"
    if ! printf '%s' "${old_heartbeats_json}" | grep -q "purge-smoke-${now}"; then
      purged=1
      break
    fi
    sleep 0.25
  done
  if [[ "${purged}" != "1" ]]; then
    echo "Heartbeat purge job did not delete old heartbeat" >&2
    exit 1
  fi

  curl -fsS \
    -X PUT \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d '{"timezone":"UTC","timeout_minutes":15,"writes_only":false,"has_public_profile":true,"country":"US","heartbeat_retention_days":365}' \
    "${api_base}/api/v1/users/current" >/dev/null

  user_retention_entity="/tmp/user-retention-smoke-${now}/old.go"
  docker compose exec -T postgres psql -U stint -d stint \
    -c "insert into heartbeats (user_id, entity, type, category, time, project, language, machine_name) values ('${user_id}', '${user_retention_entity}', 'file', 'coding', ${old_time}, 'user-retention-smoke', 'Go', 'local') on conflict do nothing;" >/dev/null

  curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/heartbeats?date=${old_date}" | grep -q "user-retention-smoke-${now}"

  curl -fsS -X POST "${api_base}/api/v1/dev/jobs/heartbeats-purge?retention_days=0" >/dev/null

  user_retention_purged=0
  for _ in $(seq 1 20); do
    old_heartbeats_json="$(curl -fsS \
      -H "Authorization: Bearer ${api_key}" \
      "${api_base}/api/v1/users/current/heartbeats?date=${old_date}")"
    if ! printf '%s' "${old_heartbeats_json}" | grep -q "user-retention-smoke-${now}"; then
      user_retention_purged=1
      break
    fi
    sleep 0.25
  done
  if [[ "${user_retention_purged}" != "1" ]]; then
    echo "Per-user heartbeat retention did not delete old heartbeat" >&2
    exit 1
  fi

  curl -fsS \
    -X PUT \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d '{"timezone":"UTC","timeout_minutes":15,"writes_only":false,"has_public_profile":true,"country":"US","heartbeat_retention_days":0}' \
    "${api_base}/api/v1/users/current" >/dev/null
fi

stats_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/stats/last_7_days")"

grep -q '"project_ai"' <<<"${stats_json}"
grep -q '"name":"stint"' <<<"${stats_json}"
grep -q '"ai_line_changes":' <<<"${stats_json}"
grep -q '"session_count":' <<<"${stats_json}"
grep -q '"human_review_percentage":' <<<"${stats_json}"
grep -q '"follow_up_edits":' <<<"${stats_json}"
grep -q '"prompt_count":' <<<"${stats_json}"
grep -q '"median_prompt_length":' <<<"${stats_json}"
grep -q '"name":"Codex".*"session_count":1' <<<"${stats_json}"

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/stats/last_30_days" >/dev/null

calendar_month="$(date -u -d "@${now}" +%Y-%m)"
curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/stats/${calendar_month}" | grep -q "\"range\":\"${calendar_month}\""

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/stats" | grep -q '"last_year"'
curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/stats" | grep -q '"all_time"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/stats/all_time" | grep -q '"range":"all_time"'

curl -fsS "${api_base}/api/v1/users/local-dev" | grep -q '"username":"local-dev"'
curl -fsS "${api_base}/api/v1/users/local-dev/stats/last_7_days" | grep -q '"range":"last_7_days"'
curl -fsS "${api_base}/api/v1/users/local-dev/stats/all_time" | grep -q '"range":"all_time"'
curl -fsS "${api_base}/api/v1/users/${user_id}/stats?callback=smokePublicStats" | grep -q '^smokePublicStats('
curl -fsS "${api_base}/api/v1/users/local-dev/summaries?start=$(date -u +%F)&end=$(date -u +%F)" | grep -q '"grand_total"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/status_bar/today" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/status_bar/today" | grep -q '"cached":true'

writes_seed_json="$(curl -fsS -X POST "${api_base}/api/v1/dev/seed-key?github_id=9002&username=writes-only-smoke")"
writes_api_key="$(printf '%s' "${writes_seed_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
if [[ -z "${writes_api_key}" ]]; then
  echo "Could not create writes-only smoke user" >&2
  exit 1
fi

curl -fsS \
  -X PUT \
  -H "Authorization: Bearer ${writes_api_key}" \
  -H "Content-Type: application/json" \
  -d '{"timezone":"UTC","timeout_minutes":15,"writes_only":true,"has_public_profile":false,"heartbeat_retention_days":0}' \
  "${api_base}/api/v1/users/current" >/dev/null

write_now="$(date -u -d "$(date -u +%F) 12:00:00" +%s)"
curl -fsS \
  -H "Authorization: Bearer ${writes_api_key}" \
  -H "Content-Type: application/json" \
  -H "User-Agent: wakatime/v1.102.1 (linux-amd64) go1.23.0 vscode/1.89.0 vscode-wakatime/24.3.0" \
  -d "[{\"entity\":\"/tmp/writes-only-${now}/read.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":${write_now},\"project\":\"writes-drop\",\"language\":\"Go\",\"machine_name\":\"local\",\"is_write\":false},{\"entity\":\"/tmp/writes-only-${now}/read.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((write_now + 600)),\"project\":\"writes-drop\",\"language\":\"Go\",\"machine_name\":\"local\",\"is_write\":false},{\"entity\":\"/tmp/writes-only-${now}/write.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((write_now + 1200)),\"project\":\"writes-keep\",\"language\":\"Go\",\"machine_name\":\"local\",\"is_write\":true},{\"entity\":\"/tmp/writes-only-${now}/write.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((write_now + 1800)),\"project\":\"writes-keep\",\"language\":\"Go\",\"machine_name\":\"local\",\"is_write\":true}]" \
  "${api_base}/api/v1/users/current/heartbeats.bulk" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${writes_api_key}" \
  "${api_base}/api/v1/users/current/heartbeats?date=$(date -u +%F)" | grep -q "writes-drop"

writes_stats="$(current_stats_until_contains "writes-keep" "${writes_api_key}")"
if grep -q "writes-drop" <<<"${writes_stats}"; then
  echo "writes_only stats included a non-write heartbeat" >&2
  exit 1
fi

writes_durations="$(curl -fsS \
  -H "Authorization: Bearer ${writes_api_key}" \
  "${api_base}/api/v1/users/current/durations?date=$(date -u +%F)")"
grep -q "writes-keep" <<<"${writes_durations}"
if grep -q "writes-drop" <<<"${writes_durations}"; then
  echo "writes_only durations included a non-write heartbeat" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${writes_api_key}" \
  "${api_base}/api/v1/users/current/status_bar/today" | grep -q "writes-keep"

curl -fsS \
  -X DELETE \
  -H "Authorization: Bearer ${writes_api_key}" \
  -H "Content-Type: application/json" \
  -d '{"confirmation":"DELETE"}' \
  "${api_base}/api/v1/users/current" >/dev/null

curl -fsS -X POST "${api_base}/api/v1/dev/jobs/leaderboard-update?range=last_7_days" >/dev/null

leaderboard_done=0
for _ in $(seq 1 20); do
  leaders_json="$(curl -fsS "${api_base}/api/v1/leaders")"
  if printf '%s' "${leaders_json}" | grep -q '"username":"local-dev"'; then
    leaderboard_done=1
    break
  fi
  sleep 0.25
done
if [[ "${leaderboard_done}" != "1" ]]; then
  echo "Leaderboard update job did not refresh public leaders" >&2
  exit 1
fi

leaders_go_json="$(curl -fsS "${api_base}/api/v1/leaders?language=Go")"
printf '%s' "${leaders_go_json}" | grep -q '"language":"Go"'
printf '%s' "${leaders_go_json}" | grep -q '"username":"local-dev"'

leaders_us_json="$(curl -fsS "${api_base}/api/v1/leaders?country=US")"
printf '%s' "${leaders_us_json}" | grep -q '"country":"US"'
printf '%s' "${leaders_us_json}" | grep -q '"username":"local-dev"'
printf '%s' "${leaders_us_json}" | grep -q '"country":"US"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/projects" >/dev/null

project_detail_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/projects/stint")"
printf '%s' "${project_detail_json}" | grep -q '"branches"'
printf '%s' "${project_detail_json}" | grep -q '"name":"main"'
printf '%s' "${project_detail_json}" | grep -q '"dependencies"'
printf '%s' "${project_detail_json}" | grep -q '"name":"pgx"'
curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/projects/stint?range=last_year" | grep -q '"range":"last_year"'

project_commits_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/projects/stint/commits?branch=main")"
printf '%s' "${project_commits_json}" | grep -q "\"hash\":\"${smoke_commit_hash}\""
printf '%s' "${project_commits_json}" | grep -q '"total_seconds":300'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/projects/stint/commits/${smoke_commit_hash}" | grep -q "\"hash\":\"${smoke_commit_hash}\""

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/machine_names" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/projects/last_30_days" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/dependencies/last_30_days" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/stats/last_30_days" | grep -q '"total_seconds"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/hours/last_30_days" | grep -q '"label":"00:00"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/weekdays/last_30_days" | grep -q '"active_days"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/daily_average_trend/last_30_days" | grep -q '"average_seconds"'

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/insights/ai_agents/last_30_days" >/dev/null

heartbeats_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/heartbeats?date=$(date -u +%F)")"

heartbeat_id="$(printf '%s' "${heartbeats_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -n 1)"

if [[ -n "${heartbeat_id}" ]]; then
  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d "{\"date\":\"$(date -u +%F)\",\"ids\":[\"${heartbeat_id}\"]}" \
    "${api_base}/api/v1/users/current/heartbeats.bulk" >/dev/null
fi

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/all_time_since_today" >/dev/null

ignored_day="sunday"
if [[ "$(date -u +%u)" == "7" ]]; then
  ignored_day="monday"
fi

invalid_goal_status="$(curl -sS -o /tmp/stint-invalid-goal-response.json -w '%{http_code}' \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"title":"Invalid smoke goal","delta":"month","seconds":300}' \
  "${api_base}/api/v1/users/current/goals")"
if [[ "${invalid_goal_status}" != "400" ]]; then
  echo "Expected invalid goal delta to return 400, got ${invalid_goal_status}" >&2
  cat /tmp/stint-invalid-goal-response.json >&2
  exit 1
fi
grep -q '"delta must be day or week' /tmp/stint-invalid-goal-response.json

goal_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Smoke goal\",\"delta\":\"day\",\"seconds\":300,\"projects\":[\"stint\"],\"languages\":[\"Go\"],\"editors\":[\"vscode\"],\"ignore_days\":[\"${ignored_day}\"],\"ignore_zero_days\":false,\"improve_by_percent\":null,\"is_enabled\":true,\"is_inverse\":false,\"is_snoozed\":false}" \
  "${api_base}/api/v1/users/current/goals")"

goal_id="$(printf '%s' "${goal_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"

if [[ -n "${goal_id}" ]]; then
  goal_detail="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/goals/${goal_id}")"
  case "${goal_detail}" in
    *'"editors":["vscode"]'*'"ignore_days":["'"${ignored_day}"'"]'*) ;;
    *)
      echo "Advanced goal fields did not round-trip" >&2
      exit 1
      ;;
  esac

  goal_disabled="$(curl -fsS \
    -X PUT \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d "{\"title\":\"Smoke goal\",\"delta\":\"day\",\"seconds\":300,\"projects\":[\"stint\"],\"languages\":[\"Go\"],\"editors\":[\"vscode\"],\"ignore_days\":[\"${ignored_day}\"],\"ignore_zero_days\":false,\"improve_by_percent\":null,\"is_enabled\":false,\"is_inverse\":false,\"is_snoozed\":false}" \
    "${api_base}/api/v1/users/current/goals/${goal_id}")"
  printf '%s' "${goal_disabled}" | grep -q '"is_enabled":false'

  curl -fsS \
    -X PUT \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d "{\"title\":\"Smoke goal updated\",\"delta\":\"day\",\"seconds\":300,\"projects\":[\"stint\"],\"languages\":[\"Go\"],\"editors\":[\"vscode\"],\"ignore_days\":[\"${ignored_day}\"],\"ignore_zero_days\":false,\"improve_by_percent\":null,\"is_enabled\":true,\"is_inverse\":false,\"is_snoozed\":false}" \
    "${api_base}/api/v1/users/current/goals/${goal_id}" | grep -q '"is_enabled":true'
  if [[ -n "${wakatime_cli_bin:-}" && -n "${cli_home:-}" ]]; then
    cli_goal_output="$(WAKATIME_HOME="${cli_home}" "${wakatime_cli_bin}" \
      --today-goal "${goal_id}" \
      --config "${cli_home}/.wakatime.cfg" \
      --verbose)"
    if [[ -z "${cli_goal_output}" ]]; then
      echo "wakatime-cli --today-goal returned empty output" >&2
      exit 1
    fi
  fi
fi

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -H "User-Agent: wakatime/v1.102.1 (linux-amd64) go1.23.0 vscode/1.89.0 vscode-wakatime/24.3.0" \
  -d "[{\"entity\":\"/tmp/goal-smoke-${now}/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((now + 700)),\"project\":\"stint\",\"language\":\"Go\",\"machine_name\":\"local\"},{\"entity\":\"/tmp/goal-smoke-${now}/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((now + 1000)),\"project\":\"stint\",\"language\":\"Go\",\"machine_name\":\"local\"}]" \
  "${api_base}/api/v1/users/current/heartbeats.bulk" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/goals" >/dev/null

if [[ -n "${goal_id}" ]]; then
  curl -fsS -X POST "${api_base}/api/v1/dev/jobs/goals-evaluate?now_unix=$((now + 86400))" >/dev/null

  if [[ "${local_compose}" == "1" ]]; then
    goal_evaluated=0
    for _ in $(seq 1 20); do
      goal_complete="$(docker compose exec -T postgres psql -U stint -d stint -tAc "select is_complete from goal_evaluations where user_id='${user_id}' and goal_id='${goal_id}' order by evaluated_at desc limit 1;")"
      if [[ "${goal_complete}" == "t" ]]; then
        goal_evaluated=1
        break
      fi
      sleep 0.25
    done
    if [[ "${goal_evaluated}" != "1" ]]; then
      echo "Goal evaluation job did not mark the smoke goal complete" >&2
      exit 1
    fi
  fi

  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/goals/${goal_id}" >/dev/null
fi

invalid_external_status="$(curl -sS -o /tmp/stint-invalid-external-response.json -w "%{http_code}" \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"external_id\":\"invalid-smoke-${now}\",\"provider\":\"manual\",\"entity\":\"Planning\",\"start_time\":$((now - 900)),\"end_time\":${now},\"project\":\"external-invalid-${now}\"}" \
  "${api_base}/api/v1/users/current/external_durations")"
if [[ "${invalid_external_status}" != "400" ]]; then
  echo "Expected invalid external duration to return 400, got ${invalid_external_status}" >&2
  cat /tmp/stint-invalid-external-response.json >&2
  exit 1
fi
grep -q '"type is required' /tmp/stint-invalid-external-response.json

external_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"external_id\":\"smoke-${now}\",\"provider\":\"manual\",\"entity\":\"Planning\",\"type\":\"app\",\"category\":\"planning\",\"start_time\":$((now - 900)),\"end_time\":${now},\"project\":\"external-smoke-${now}\",\"language\":\"Markdown\"}" \
  "${api_base}/api/v1/users/current/external_durations")"

external_id="$(printf '%s' "${external_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"

external_bulk_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "[{\"external_id\":\"bulk-smoke-${now}\",\"provider\":\"manual\",\"entity\":\"Bulk planning\",\"type\":\"app\",\"category\":\"planning\",\"start_time\":$((now - 600)),\"end_time\":${now},\"project\":\"external-bulk-${now}\",\"language\":\"Markdown\"}]" \
  "${api_base}/api/v1/users/current/external_durations.bulk")"
printf '%s' "${external_bulk_json}" | grep -q '"status":201'
external_bulk_id="$(printf '%s' "${external_bulk_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [[ -z "${external_bulk_id}" ]]; then
  echo "Could not create external duration through bulk endpoint" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/external_durations" >/dev/null

post_external_key_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Post-external smoke"}' \
  "${api_base}/api/v1/api_keys")"
api_key="$(printf '%s' "${post_external_key_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
if [[ -z "${api_key}" ]]; then
  echo "Could not rotate smoke API key after read limit exercise" >&2
  exit 1
fi

external_stats="$(current_stats_until_contains "external-bulk-${now}")"
grep -q "external-smoke-${now}" <<<"${external_stats}"
grep -q "external-bulk-${now}" <<<"${external_stats}"
grep -q '"Markdown"' <<<"${external_stats}"

external_goal_duration_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"external_id\":\"goal-smoke-${now}\",\"provider\":\"manual\",\"entity\":\"Goal planning\",\"type\":\"app\",\"category\":\"planning\",\"start_time\":$((now + 700)),\"end_time\":$((now + 1600)),\"project\":\"external-goal-${now}\",\"language\":\"Markdown\"}" \
  "${api_base}/api/v1/users/current/external_durations")"
external_goal_duration_id="$(printf '%s' "${external_goal_duration_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"

external_goal_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"External duration goal\",\"delta\":\"day\",\"seconds\":900,\"projects\":[\"external-goal-${now}\"],\"languages\":[\"Markdown\"],\"is_enabled\":true}" \
  "${api_base}/api/v1/users/current/goals")"
external_goal_id="$(printf '%s' "${external_goal_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [[ -n "${external_goal_id}" && "${local_compose}" == "1" ]]; then
  curl -fsS -X POST "${api_base}/api/v1/dev/jobs/goals-evaluate?now_unix=$((now + 86400))" >/dev/null
  external_goal_evaluated=0
  for _ in $(seq 1 20); do
    external_goal_actual="$(docker compose exec -T postgres psql -U stint -d stint -tAc "select actual_seconds from goal_evaluations where user_id='${user_id}' and goal_id='${external_goal_id}' order by evaluated_at desc limit 1;")"
    if [[ "${external_goal_actual}" == "900" ]]; then
      external_goal_evaluated=1
      break
    fi
    sleep 0.25
  done
  if [[ "${external_goal_evaluated}" != "1" ]]; then
    echo "Goal evaluation job did not include external duration" >&2
    exit 1
  fi
fi
if [[ -n "${external_goal_id}" ]]; then
  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/goals/${external_goal_id}" >/dev/null
fi

external_delete_ids=()
if [[ -n "${external_id}" ]]; then
  external_delete_ids+=("\"${external_id}\"")
fi
if [[ -n "${external_bulk_id}" ]]; then
  external_delete_ids+=("\"${external_bulk_id}\"")
fi
if [[ -n "${external_goal_duration_id:-}" ]]; then
  external_delete_ids+=("\"${external_goal_duration_id}\"")
fi
if [[ "${#external_delete_ids[@]}" -gt 0 ]]; then
  external_delete_payload="$(IFS=,; printf '{"ids":[%s]}' "${external_delete_ids[*]}")"
  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d "${external_delete_payload}" \
    "${api_base}/api/v1/users/current/external_durations.bulk" >/dev/null
fi

daily_dump_old_time=$((now - (30 * 86400)))
daily_dump_old_date="$(date -u -d "@${daily_dump_old_time}" +%F)"
curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -H "User-Agent: wakatime/v1.102.1 (linux-amd64) go1.23.0 vscode/1.89.0 vscode-wakatime/24.3.0" \
  -d "{\"entity\":\"/tmp/daily-dump-smoke-${now}/old.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":${daily_dump_old_time},\"project\":\"daily-dump-old-${now}\",\"language\":\"Go\",\"machine_name\":\"local\"}" \
  "${api_base}/api/v1/users/current/heartbeats" >/dev/null

dump_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"type":"heartbeats"}' \
  "${api_base}/api/v1/users/current/data_dumps")"

dump_id="$(printf '%s' "${dump_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
dump_url=""

for _ in $(seq 1 20); do
  dumps_json="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/data_dumps")"
  dump_row="$(printf '%s' "${dumps_json}" | tr '{' '\n' | grep "\"id\":\"${dump_id}\"" | head -n 1 || true)"
  dump_url="$(printf '%s' "${dump_row}" | sed -n 's/.*"download_url":"\([^"]*\)".*/\1/p' | sed 's#\\/#/#g')"
  if [[ -n "${dump_url}" ]]; then
    break
  fi
  sleep 0.25
done

if [[ -n "${dump_url}" ]]; then
  dump_payload="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}${dump_url}")"
  printf '%s' "${dump_payload}" | assert_json_array "Heartbeat data dump"
  printf '%s' "${dump_payload}" | grep -q "daily-dump-old-${now}"
else
  echo "Data dump did not complete" >&2
  exit 1
fi

daily_dump_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"type":"daily"}' \
  "${api_base}/api/v1/users/current/data_dumps")"

daily_dump_id="$(printf '%s' "${daily_dump_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
daily_dump_url=""

for _ in $(seq 1 20); do
  dumps_json="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/data_dumps")"
  dump_row="$(printf '%s' "${dumps_json}" | tr '{' '\n' | grep "\"id\":\"${daily_dump_id}\"" | head -n 1 || true)"
  daily_dump_url="$(printf '%s' "${dump_row}" | sed -n 's/.*"download_url":"\([^"]*\)".*/\1/p' | sed 's#\\/#/#g')"
  if [[ -n "${daily_dump_url}" ]]; then
    break
  fi
  sleep 0.25
done

if [[ -n "${daily_dump_url}" ]]; then
  daily_dump_payload="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}${daily_dump_url}")"
  printf '%s' "${daily_dump_payload}" | assert_json_array "Daily data dump"
  printf '%s' "${daily_dump_payload}" | grep -q '"grand_total"'
  printf '%s' "${daily_dump_payload}" | grep -q '"projects"'
  printf '%s' "${daily_dump_payload}" | grep -q "\"date\":\"${daily_dump_old_date}\""
else
  echo "Daily data dump did not complete" >&2
  exit 1
fi

invalid_dump_status="$(curl -sS -o /tmp/stint-invalid-dump-response.json -w '%{http_code}' \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"type":"summaries"}' \
  "${api_base}/api/v1/users/current/data_dumps")"
if [[ "${invalid_dump_status}" != "400" ]]; then
  echo "Expected invalid data dump type to return 400, got ${invalid_dump_status}" >&2
  cat /tmp/stint-invalid-dump-response.json >&2
  exit 1
fi
grep -q '"unsupported data dump type' /tmp/stint-invalid-dump-response.json

invalid_board_status="$(curl -sS -o /tmp/stint-invalid-board-response.json -w '%{http_code}' \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Invalid leaderboard","time_range":"yesterday"}' \
  "${api_base}/api/v1/users/current/leaderboards")"
if [[ "${invalid_board_status}" != "400" ]]; then
  echo "Expected invalid leaderboard time_range to return 400, got ${invalid_board_status}" >&2
  cat /tmp/stint-invalid-board-response.json >&2
  exit 1
fi
grep -q '"time_range must be a supported stats range' /tmp/stint-invalid-board-response.json

invalid_board_name_status="$(curl -sS -o /tmp/stint-invalid-board-response.json -w '%{http_code}' \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"   ","time_range":"last_7_days"}' \
  "${api_base}/api/v1/users/current/leaderboards")"
if [[ "${invalid_board_name_status}" != "400" ]]; then
  echo "Expected invalid leaderboard name to return 400, got ${invalid_board_name_status}" >&2
  cat /tmp/stint-invalid-board-response.json >&2
  exit 1
fi
grep -q 'leaderboard name is required' /tmp/stint-invalid-board-response.json

board_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Smoke leaderboard","time_range":"last_7_days"}' \
  "${api_base}/api/v1/users/current/leaderboards")"

board_id="$(printf '%s' "${board_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
member_seed_json="$(curl -fsS -X POST "${api_base}/api/v1/dev/seed-key?github_id=9003&username=leaderboard-member-smoke")"
member_key="$(printf '%s' "${member_seed_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
member_id="$(printf '%s' "${member_seed_json}" | sed -n 's/.*"user":{"id":"\([^"]*\)".*/\1/p')"

if [[ -z "${member_key}" || -z "${member_id}" ]]; then
  echo "Could not create leaderboard member seed user" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/leaderboards" >/dev/null

if [[ -n "${board_id}" ]]; then
  curl -fsS \
    -X POST \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d '{"username":"leaderboard-member-smoke"}' \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}/members" >/dev/null

  board_members_json="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}")"
  if ! printf '%s' "${board_members_json}" | grep -q '"username":"leaderboard-member-smoke"'; then
    echo "Leaderboard member was not added" >&2
    exit 1
  fi

  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}/members/${member_id}" >/dev/null

  board_members_json="$(curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}")"
  if printf '%s' "${board_members_json}" | grep -q '"username":"leaderboard-member-smoke"'; then
    echo "Leaderboard member was not removed" >&2
    exit 1
  fi

  curl -fsS \
    -X PUT \
    -H "Authorization: Bearer ${api_key}" \
    -H "Content-Type: application/json" \
    -d '{"name":"Smoke leaderboard updated","time_range":"last_30_days"}' \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}" >/dev/null
  curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}" >/dev/null
  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/leaderboards/${board_id}" >/dev/null
fi

curl -fsS \
  -X DELETE \
  -H "Authorization: Bearer ${member_key}" \
  -H "Content-Type: application/json" \
  -d '{"confirmation":"DELETE"}' \
  "${api_base}/api/v1/users/current" >/dev/null

retro_project="retro-${now}"
curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -H "User-Agent: wakatime/v1.102.1 (linux-amd64) go1.23.0 vscode/1.89.0 vscode-wakatime/24.3.0" \
  -d "{\"entity\":\"/tmp/retro-smoke-${now}/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((now + 550)),\"project\":\"original\",\"language\":\"Go\",\"machine_name\":\"local\"}" \
  "${api_base}/api/v1/users/current/heartbeats" >/dev/null

invalid_rule_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '[{"action":"change","source":"entity","operation":"regex","source_value":"[","priority":1,"destinations":[{"destination":"project","destination_value":"rewritten"}]}]' \
  "${api_base}/api/v1/users/current/custom_rules")"
if [[ "${invalid_rule_status}" != "400" ]]; then
  echo "Invalid regex custom rule returned ${invalid_rule_status}, expected 400" >&2
  exit 1
fi

invalid_rule_destination_status="$(curl -sS -o /tmp/stint-invalid-custom-rule-response.json -w "%{http_code}" \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '[{"action":"change","source":"entity","operation":"contains","source_value":"retro","priority":1,"destinations":[{"destination":"project","destination_value":""}]}]' \
  "${api_base}/api/v1/users/current/custom_rules")"
if [[ "${invalid_rule_destination_status}" != "400" ]]; then
  echo "Invalid destination custom rule returned ${invalid_rule_destination_status}, expected 400" >&2
  cat /tmp/stint-invalid-custom-rule-response.json >&2
  exit 1
fi
grep -q '"change custom rules require destination and destination_value' /tmp/stint-invalid-custom-rule-response.json

curl -fsS \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "[{\"action\":\"change\",\"source\":\"entity\",\"operation\":\"regex\",\"source_value\":\"rule-smoke/.+\\\\.go\",\"priority\":1,\"destinations\":[{\"destination\":\"project\",\"destination_value\":\"rewritten\"}]},{\"action\":\"change\",\"source\":\"entity\",\"operation\":\"contains\",\"source_value\":\"retro-smoke-${now}\",\"priority\":2,\"destinations\":[{\"destination\":\"project\",\"destination_value\":\"${retro_project}\"}]}]" \
  "${api_base}/api/v1/users/current/custom_rules" >/dev/null

custom_rules_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/custom_rules")"
custom_rule_id="$(printf '%s' "${custom_rules_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -n 1)"
if [[ -z "${custom_rule_id}" ]]; then
  echo "Could not read custom rule id" >&2
  exit 1
fi

retro_done=0
for _ in $(seq 1 20); do
  if curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/projects/${retro_project}" >/dev/null 2>&1; then
    retro_done=1
    break
  fi
  sleep 0.25
done
if [[ "${retro_done}" != "1" ]]; then
  echo "Retroactive custom rule did not complete" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -H "User-Agent: wakatime/v1.102.1 (linux-amd64) go1.23.0 vscode/1.89.0 vscode-wakatime/24.3.0" \
  -d "{\"entity\":\"/tmp/rule-smoke/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((now + 600)),\"project\":\"original\",\"language\":\"Go\",\"machine_name\":\"local\"}" \
  "${api_base}/api/v1/users/current/heartbeats" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/projects/rewritten" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/custom_rules_progress" | grep -q '"status":"Completed"'

curl -fsS \
  -X DELETE \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/custom_rules/${custom_rule_id}" >/dev/null
custom_rules_after_delete="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/custom_rules")"
if printf '%s' "${custom_rules_after_delete}" | grep -q "${custom_rule_id}"; then
  echo "Deleted custom rule was still listed" >&2
  exit 1
fi

curl -fsS \
  -X DELETE \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/custom_rules_progress" | grep -q '"status":"Aborted"'

import_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"data\":[{\"entity\":\"/tmp/import-smoke/main.go\",\"type\":\"file\",\"category\":\"coding\",\"time\":$((now + 900)),\"project\":\"imported\",\"language\":\"Go\",\"machine_name\":\"local\"}]}" \
  "${api_base}/api/v1/users/current/imports/wakatime")"

printf '%s' "${import_json}" | grep -Eq '"status":"(Queued|Completed)"'

import_done=0
for _ in $(seq 1 20); do
  if curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/projects/imported" >/dev/null 2>&1; then
    import_done=1
    break
  fi
  sleep 0.25
done
if [[ "${import_done}" != "1" ]]; then
  echo "WakaTime import job did not insert imported project" >&2
  exit 1
fi

heartbeats_import_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"heartbeats\":[{\"entity\":\"/tmp/import-smoke/wrapped.go\",\"category\":\"coding\",\"time\":$((now + 960)),\"project\":\"imported-heartbeats\",\"language\":\"Go\",\"machine_name\":\"local\"}]}" \
  "${api_base}/api/v1/users/current/imports/wakatime")"

printf '%s' "${heartbeats_import_json}" | grep -Eq '"status":"(Queued|Completed)"'

heartbeats_import_done=0
for _ in $(seq 1 20); do
  if curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/projects/imported-heartbeats" >/dev/null 2>&1; then
    heartbeats_import_done=1
    break
  fi
  sleep 0.25
done
if [[ "${heartbeats_import_done}" != "1" ]]; then
  echo "WakaTime heartbeats wrapper import job did not insert imported-heartbeats project" >&2
  exit 1
fi

import_tmp="$(mktemp -d)"
printf '{"data":[{"entity":"/tmp/import-smoke/compressed.go","type":"file","category":"coding","time":%s,"project":"imported-gzip","language":"Go","machine_name":"local"}]}' "$((now + 990))" >"${import_tmp}/wakatime.json"
gzip -c "${import_tmp}/wakatime.json" >"${import_tmp}/wakatime.json.gz"

gzip_import_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -F "file=@${import_tmp}/wakatime.json.gz;type=application/gzip" \
  "${api_base}/api/v1/users/current/imports/wakatime")"

printf '%s' "${gzip_import_json}" | grep -Eq '"status":"(Queued|Completed)"'

gzip_import_done=0
for _ in $(seq 1 20); do
  if curl -fsS \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/users/current/projects/imported-gzip" >/dev/null 2>&1; then
    gzip_import_done=1
    break
  fi
  sleep 0.25
done
if [[ "${gzip_import_done}" != "1" ]]; then
  echo "WakaTime gzip import job did not insert imported-gzip project" >&2
  exit 1
fi

invalid_ai_cost_status="$(curl -sS -o /tmp/stint-invalid-ai-cost-response.json -w "%{http_code}" \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '[{"agent":"Codex","input_cost_per_million_cents":-1,"output_cost_per_million_cents":400}]' \
  "${api_base}/api/v1/users/current/ai_costs")"
if [[ "${invalid_ai_cost_status}" != "400" ]]; then
  echo "Invalid AI cost settings returned ${invalid_ai_cost_status}, expected 400" >&2
  cat /tmp/stint-invalid-ai-cost-response.json >&2
  exit 1
fi
grep -q '"AI cost settings must be non-negative' /tmp/stint-invalid-ai-cost-response.json

curl -fsS \
  -X PUT \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '[{"agent":"Codex","input_cost_per_million_cents":100,"output_cost_per_million_cents":400}]' \
  "${api_base}/api/v1/users/current/ai_costs" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/ai_costs" >/dev/null

invalid_share_status="$(curl -sS -o /tmp/stint-invalid-share-token-response.json -w "%{http_code}" \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"   "}' \
  "${api_base}/api/v1/users/current/share_tokens")"
if [[ "${invalid_share_status}" != "400" ]]; then
  echo "Expected invalid share token name to return 400, got ${invalid_share_status}" >&2
  cat /tmp/stint-invalid-share-token-response.json >&2
  exit 1
fi
grep -q 'share token name is required' /tmp/stint-invalid-share-token-response.json

share_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Smoke share"}' \
  "${api_base}/api/v1/users/current/share_tokens")"

share_id="$(printf '%s' "${share_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
share_token="$(printf '%s' "${share_json}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
user_id="$(printf '%s' "${seed_json}" | sed -n 's/.*"user":{"id":"\([^"]*\)".*/\1/p')"

if [[ -z "${share_id}" || -z "${share_token}" || -z "${user_id}" ]]; then
  echo "Could not create share token" >&2
  exit 1
fi

curl -fsS \
  "${api_base}/api/v1/users/${user_id}/share/${share_token}/stats?range=last_7_days" >/dev/null

jsonp="$(curl -fsS \
  "${api_base}/api/v1/users/${user_id}/share/${share_token}/summaries?callback=Stint.render")"
case "${jsonp}" in
  Stint.render*) ;;
  *)
    echo "JSONP share endpoint did not wrap response" >&2
    exit 1
    ;;
esac

curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/share_tokens" >/dev/null

curl -fsS \
  -X DELETE \
  -H "Authorization: Bearer ${api_key}" \
  "${api_base}/api/v1/users/current/share_tokens/${share_id}" >/dev/null

oauth_redirect="${api_base}/oauth/smoke/callback"
invalid_oauth_app_status="$(curl -sS -o /tmp/stint-invalid-oauth-app-response.json -w "%{http_code}" \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"Invalid OAuth app","redirect_uris":["ftp://example.com/callback"],"scopes":["read_stats"]}' \
  "${api_base}/api/v1/oauth/apps")"
if [[ "${invalid_oauth_app_status}" != "400" ]]; then
  echo "Expected invalid OAuth app redirect URI to return 400, got ${invalid_oauth_app_status}" >&2
  cat /tmp/stint-invalid-oauth-app-response.json >&2
  exit 1
fi
grep -q '"redirect URIs must use http or https' /tmp/stint-invalid-oauth-app-response.json

invalid_oauth_app_name_status="$(curl -sS -o /tmp/stint-invalid-oauth-app-response.json -w "%{http_code}" \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d '{"name":"   ","redirect_uris":["http://localhost:8080/callback"],"scopes":["read_stats"]}' \
  "${api_base}/api/v1/oauth/apps")"
if [[ "${invalid_oauth_app_name_status}" != "400" ]]; then
  echo "Expected invalid OAuth app name to return 400, got ${invalid_oauth_app_name_status}" >&2
  cat /tmp/stint-invalid-oauth-app-response.json >&2
  exit 1
fi
grep -q 'OAuth app name is required' /tmp/stint-invalid-oauth-app-response.json

oauth_app_json="$(curl -fsS \
  -H "Authorization: Bearer ${api_key}" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Smoke OAuth app\",\"redirect_uris\":[\"${oauth_redirect}\"],\"scopes\":[\"read_stats\",\"read_summaries\",\"write_heartbeats\"]}" \
  "${api_base}/api/v1/oauth/apps")"

client_id="$(printf '%s' "${oauth_app_json}" | sed -n 's/.*"client_id":"\([^"]*\)".*/\1/p')"
client_secret="$(printf '%s' "${oauth_app_json}" | sed -n 's/.*"client_secret":"\([^"]*\)".*/\1/p')"
oauth_app_id="$(printf '%s' "${oauth_app_json}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"

if [[ -z "${client_id}" || -z "${client_secret}" ]]; then
  echo "Could not create OAuth app" >&2
  exit 1
fi

oauth_seed_json="$(curl -fsS -c "${cookie_jar}" -X POST "${api_base}/api/v1/dev/seed-key?github_id=$((now + 910000000))&username=oauth-smoke-${now}")"
oauth_user_id="$(printf '%s' "${oauth_seed_json}" | sed -n 's/.*"user":{"id":"\([^"]*\)".*/\1/p')"
if [[ -z "${oauth_user_id}" ]]; then
  echo "Could not create per-run OAuth smoke user" >&2
  exit 1
fi

curl -fsS -b "${cookie_jar}" -D "${headers_file}" -o /dev/null \
  -X POST \
  --data-urlencode "response_type=code" \
  --data-urlencode "client_id=${client_id}" \
  --data-urlencode "redirect_uri=${oauth_redirect}" \
  --data-urlencode "scope=read_stats read_summaries write_heartbeats" \
  --data-urlencode "state=smoke" \
  --data-urlencode "decision=allow" \
  "${api_base}/oauth/authorize"

oauth_code="$(sed -n 's/[Ll]ocation:.*[?&]code=\([^&[:space:]]*\).*/\1/p' "${headers_file}" | tr -d '\r' | head -n 1)"
if [[ -z "${oauth_code}" ]]; then
  echo "Could not read OAuth authorization code" >&2
  exit 1
fi

token_json="$(curl -fsS \
  -u "${client_id}:${client_secret}" \
  --data-urlencode "grant_type=authorization_code" \
  --data-urlencode "code=${oauth_code}" \
  --data-urlencode "redirect_uri=${oauth_redirect}" \
  "${api_base}/oauth/token")"

oauth_access_token="$(printf '%s' "${token_json}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
oauth_refresh_token="$(printf '%s' "${token_json}" | sed -n 's/.*"refresh_token":"\([^"]*\)".*/\1/p')"

if [[ -z "${oauth_access_token}" || -z "${oauth_refresh_token}" ]]; then
  echo "Could not exchange OAuth code" >&2
  exit 1
fi
if [[ "${oauth_access_token}" != waka_tok_* ]]; then
  echo "OAuth access token did not use waka_tok_ prefix" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${oauth_access_token}" \
  "${api_base}/api/v1/users/current" >/dev/null

oauth_goal_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${oauth_access_token}" \
  "${api_base}/api/v1/users/current/goals")"
if [[ "${oauth_goal_status}" != "403" ]]; then
  echo "OAuth token without read_goals unexpectedly accessed goals" >&2
  exit 1
fi

oauth_leaderboard_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${oauth_access_token}" \
  "${api_base}/api/v1/users/current/leaderboards")"
if [[ "${oauth_leaderboard_status}" != "403" ]]; then
  echo "OAuth token without read_private_leaderboards unexpectedly accessed private leaderboards" >&2
  exit 1
fi

curl -fsS -b "${cookie_jar}" -D "${headers_file}" -o /dev/null \
  -X POST \
  --data-urlencode "response_type=token" \
  --data-urlencode "client_id=${client_id}" \
  --data-urlencode "redirect_uri=${oauth_redirect}" \
  --data-urlencode "scope=read_stats.projects" \
  --data-urlencode "state=implicit-smoke" \
  --data-urlencode "decision=allow" \
  "${api_base}/oauth/authorize"

implicit_location="$(sed -n 's/[Ll]ocation:[[:space:]]*//p' "${headers_file}" | tr -d '\r' | head -n 1)"
implicit_access_token="$(printf '%s' "${implicit_location}" | sed -n 's/.*[#&]access_token=\([^&[:space:]]*\).*/\1/p')"
if [[ -z "${implicit_access_token}" || "${implicit_access_token}" != waka_tok_* ]]; then
  echo "Could not read implicit OAuth access token" >&2
  exit 1
fi
if printf '%s' "${implicit_location}" | grep -q 'refresh_token='; then
  echo "Implicit OAuth response unexpectedly included a refresh token" >&2
  exit 1
fi
if [[ "${local_compose}" == "1" ]]; then
  implicit_fingerprint="$(TOKEN="${implicit_access_token}" node -e 'const crypto = require("crypto"); process.stdout.write(crypto.createHash("sha256").update(process.env.TOKEN).digest("hex").slice(0, 16));')"
  implicit_refresh_null_count="$(docker compose exec -T postgres psql -U stint -d stint -tAc "select count(*) from oauth_tokens where access_token_fingerprint='${implicit_fingerprint}' and refresh_token_hash is null and refresh_token_fingerprint is null;")"
  if [[ "${implicit_refresh_null_count}" != "1" ]]; then
    echo "Implicit OAuth token unexpectedly stored refresh token material" >&2
    exit 1
  fi
fi
if ! printf '%s' "${implicit_location}" | grep -q 'state=implicit-smoke'; then
  echo "Implicit OAuth response did not preserve state" >&2
  exit 1
fi

curl -fsS \
  -H "Authorization: Bearer ${implicit_access_token}" \
  "${api_base}/api/v1/users/current/projects" >/dev/null

curl -fsS \
  -H "Authorization: Bearer ${implicit_access_token}" \
  "${api_base}/api/v1/users/current/insights/projects/last_30_days" >/dev/null

implicit_full_stats_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${implicit_access_token}" \
  "${api_base}/api/v1/users/current/stats/last_7_days")"
if [[ "${implicit_full_stats_status}" != "403" ]]; then
  echo "Granular OAuth token unexpectedly accessed full stats" >&2
  exit 1
fi

oauth_tokens=("${oauth_access_token}")
oauth_refresh_tokens=("${oauth_refresh_token}")
for token_index in $(seq 1 7); do
  curl -fsS -b "${cookie_jar}" -D "${headers_file}" -o /dev/null \
    -X POST \
    --data-urlencode "response_type=code" \
    --data-urlencode "client_id=${client_id}" \
    --data-urlencode "redirect_uri=${oauth_redirect}" \
    --data-urlencode "scope=read_stats read_summaries write_heartbeats" \
    --data-urlencode "state=retention-${token_index}" \
    --data-urlencode "decision=allow" \
    "${api_base}/oauth/authorize"

  retention_code="$(sed -n 's/[Ll]ocation:.*[?&]code=\([^&[:space:]]*\).*/\1/p' "${headers_file}" | tr -d '\r' | head -n 1)"
  retention_token_json="$(curl -fsS \
    -u "${client_id}:${client_secret}" \
    --data-urlencode "grant_type=authorization_code" \
    --data-urlencode "code=${retention_code}" \
    --data-urlencode "redirect_uri=${oauth_redirect}" \
    "${api_base}/oauth/token")"
  retention_access_token="$(printf '%s' "${retention_token_json}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"
  retention_refresh_token="$(printf '%s' "${retention_token_json}" | sed -n 's/.*"refresh_token":"\([^"]*\)".*/\1/p')"
  oauth_tokens+=("${retention_access_token}")
  oauth_refresh_tokens+=("${retention_refresh_token}")
done

oldest_token_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${oauth_tokens[0]}" \
  "${api_base}/api/v1/users/current")"
if [[ "${oldest_token_status}" != "401" ]]; then
  echo "Oldest OAuth token was not pruned after exceeding retention limit" >&2
  exit 1
fi

newest_token_status="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${oauth_tokens[7]}" \
  "${api_base}/api/v1/users/current")"
if [[ "${newest_token_status}" != "200" ]]; then
  echo "Newest OAuth token was unexpectedly rejected" >&2
  exit 1
fi

refreshed_json="$(curl -fsS \
  -u "${client_id}:${client_secret}" \
  --data-urlencode "grant_type=refresh_token" \
  --data-urlencode "refresh_token=${oauth_refresh_tokens[7]}" \
  "${api_base}/oauth/token")"
refreshed_access_token="$(printf '%s' "${refreshed_json}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')"

if [[ -z "${refreshed_access_token}" ]]; then
  echo "Could not refresh OAuth token" >&2
  exit 1
fi
if [[ "${refreshed_access_token}" != waka_tok_* ]]; then
  echo "Refreshed OAuth access token did not use waka_tok_ prefix" >&2
  exit 1
fi

if [[ "${local_compose}" == "1" ]]; then
  docker compose exec -T postgres psql -U stint -d stint \
    -c "update oauth_tokens set expires_at = now() - interval '1 second' where user_id = '${oauth_user_id}' and app_id = '${oauth_app_id}' and revoked_at is null;" >/dev/null

  expired_refresh_status="$(curl -sS -o /dev/null -w "%{http_code}" \
    -u "${client_id}:${client_secret}" \
    --data-urlencode "grant_type=refresh_token" \
    --data-urlencode "refresh_token=${oauth_refresh_tokens[6]}" \
    "${api_base}/oauth/token")"
  if [[ "${expired_refresh_status}" != "400" ]]; then
    echo "Expired OAuth refresh token returned ${expired_refresh_status}, expected 400" >&2
    exit 1
  fi
fi

curl -fsS \
  -u "${client_id}:${client_secret}" \
  --data-urlencode "token=${refreshed_access_token}" \
  "${api_base}/oauth/revoke" >/dev/null

if [[ -n "${oauth_app_id}" ]]; then
  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${api_key}" \
    "${api_base}/api/v1/oauth/apps/${oauth_app_id}" >/dev/null
fi

logout_status="$(curl -sS -b "${cookie_jar}" -c "${cookie_jar}" -D "${headers_file}" -o /dev/null -w "%{http_code}" \
  -X POST \
  "${api_base}/auth/logout")"
if [[ "${logout_status}" != "200" ]]; then
  echo "Logout returned ${logout_status}" >&2
  exit 1
fi
grep -qi "Set-Cookie: ${session_cookie_name:-stint_session}=;" "${headers_file}"
grep -qi "Set-Cookie: ${session_jwt_cookie_name:-stint_api_jwt}=;" "${headers_file}"
logged_out_status="$(curl -sS -b "${cookie_jar}" -o /dev/null -w "%{http_code}" "${api_base}/api/v1/auth/me")"
if [[ "${logged_out_status}" != "401" ]]; then
  echo "Session cookie still authorized after logout" >&2
  exit 1
fi

delete_seed_json="$(curl -fsS -X POST "${api_base}/api/v1/dev/seed-key?github_id=9001&username=delete-smoke")"
delete_api_key="$(printf '%s' "${delete_seed_json}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
if [[ -n "${delete_api_key}" ]]; then
  curl -fsS \
    -X DELETE \
    -H "Authorization: Bearer ${delete_api_key}" \
    -H "Content-Type: application/json" \
    -d '{"confirmation":"DELETE"}' \
    "${api_base}/api/v1/users/current" >/dev/null
fi
