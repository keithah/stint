# Stint for Codex CLI

Track Codex CLI activity with Stint.

## Requirements

Install and configure the Stint CLI first. Use the generated command from
Integrations so the installer writes `~/.stint.cfg` and runs `stint doctor`:

```bash
curl -fsSL https://stint.fyi/install.sh | STINT_API_URL="https://stint.fyi/api/v1" STINT_API_KEY="stint_123" sh
```

If `stint` is not on your `PATH`, set `STINT_BIN` to the absolute path of the
binary. The plugin does not install Stint from hook execution unless
`STINT_PLUGIN_AUTO_INSTALL=1` is set.

## Installing

1. Add the Stint plugin marketplace:

```bash
codex plugin marketplace add https://github.com/keithah/stint.git
```

2. Install the plugin:

```bash
codex plugin add codex-cli-stint@stint
```

3. Use Codex CLI like you normally do. Your AI coding activity will appear in Stint.

## Upgrading

```bash
codex plugin marketplace upgrade stint
```

## Troubleshooting

Logs are written to `~/.wakatime/codex-cli-stint.log`.

To check your setup:

```bash
stint doctor
stint --sync-ai-activity --ai-agent codex
```

The plugin runs on Codex `SessionEnd` and `UserPromptSubmit` hooks, then calls
`stint --sync-ai-activity --ai-agent codex` in the background.

## Privacy

Stint reads local Codex activity and sends WakaTime-shaped heartbeats to the
`api_url` in `~/.stint.cfg`. Existing `~/.wakatime.cfg` settings still work as a
compatibility fallback.
