# Stint for Codex CLI

Track Codex CLI activity with Stint.

## Requirements

Install the Stint CLI first:

```bash
curl -fsSL https://stint.fyi/install.sh | sh
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

3. Save your Stint endpoint and API key to `~/.wakatime.cfg`:

```cfg
[settings]
api_url = https://stint.fyi/api/v1
api_key = waka_123
```

4. Use Codex CLI like you normally do. Your AI coding activity will appear in Stint.

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
`api_url` in `~/.wakatime.cfg`. Configure include/exclude and privacy settings
in the same file.
