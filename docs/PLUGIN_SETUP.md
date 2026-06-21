# Editor Client Setup

Stint collects editor and agent activity through `/api/v1`. Existing WakaTime-compatible plugins can point at Stint by changing `api_url` and `api_key`, while native Stint clients can add richer model, provider, token, and cost metadata.

## 1. Create an API Key

For local development, open `http://localhost:3000/login` when running `npm run dev`, or `http://localhost:3001/login` when using Docker Compose. Choose **Create local dev key**, then open Settings and create/copy the generated Stint API key.

For a deployed instance, sign in through GitHub, open Settings, and create an API key.

## 2. Configure an Existing Editor Client

Use this in `~/.wakatime.cfg`:

```ini
[settings]
api_url = https://api.example.com/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
hide_file_names = false
timeout = 15
```

For local Compose, use:

```ini
[settings]
api_url = http://localhost:8080/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
hide_file_names = false
timeout = 15
```

For Codex or other clients using multi-destination fanout, use an `api_urls` entry instead of `api_url`:

```ini
[api_urls]
.* = https://api.example.com/api/v1|waka_00000000-0000-4000-8000-000000000000
```

Some existing clients validate `api_urls` keys more strictly than normal `api_key` settings. Stint-generated keys use `waka_<uuid>` so they work there; older bare UUID Stint keys still authenticate with the API but should be replaced for fanout configs.

## 3. Verify Ingestion

Send one heartbeat from your editor, then check:

```bash
curl -fsS -H "Authorization: Bearer waka_00000000-0000-4000-8000-000000000000" \
  "https://api.example.com/api/v1/users/current/stats/last_7_days"
```

For local development, the project smoke test exercises the same path:

```bash
scripts/smoke-wakatime.sh
```

The smoke test sends curl-based activity payloads every run. If `wakatime-cli` is installed, or `WAKATIME_CLI_BIN` points at a binary, it also sends a real CLI heartbeat, verifies the project appears, and runs `wakatime-cli --today`, `wakatime-cli --today-goal`, and `wakatime-cli --file-experts` against the local API.

## Auth Modes

Generated API keys use `waka_<uuid>` so existing editor plugins and fanout configs accept them. Bare UUID keys from older Stint builds are still accepted by the API for self-hosted migrations, but should be replaced before using `api_urls`. Keys can be supplied in all compatibility forms:

- `Authorization: Basic base64(API_KEY:)`
- `Authorization: Bearer API_KEY`
- `?api_key=API_KEY`

OAuth app tokens use `Authorization: Bearer waka_tok_...` and are scope-checked.

## Common Checks

- `api_url` must include `/api/v1`.
- The API service must be reachable from the machine running the editor plugin.
- If Stint is behind a reverse proxy, preserve the request body and `Authorization` header.
- Dashboard totals update after stats recomputation; `/status_bar/today` uses a short cache.
