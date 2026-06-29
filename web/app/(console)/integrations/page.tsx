"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { ArrowRight, Bot, Cable, Check, CheckCircle2, Clipboard, Code2, KeyRound, PlugZap, Plus, Radar, ShieldCheck, TerminalSquare } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "@/components/ui";
import { createKey, listEditors, listKeys, listUserAgents, serverMeta, type UserAgent } from "@/lib/api";

export default function IntegrationsPage() {
  return (
    <IntegrationsContent />
  );
}

function IntegrationsContent() {
  const queryClient = useQueryClient();
  const [latestKey, setLatestKey] = useState("");
  const [copied, setCopied] = useState("");
  const [selectedIntegration, setSelectedIntegration] = useState("stint-cli-config");
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const meta = useQuery({ queryKey: ["server-meta"], queryFn: serverMeta, staleTime: 60000 });
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: listKeys, });
  const editors = useQuery({ queryKey: ["editors"], queryFn: listEditors, staleTime: 3600000 });
  const userAgents = useQuery({ queryKey: ["user-agents"], queryFn: listUserAgents, staleTime: 60000 });
  const apiURL = meta.data?.data.api_url || "https://stint.fyi/api/v1";
  const displayKey = latestKey || "waka_your_stint_key";
  const keyCount = keys.data?.data.length ?? 0;
  const editorCount = editors.data?.data.length ?? 0;
  const agentRows = userAgents.data?.data ?? [];
  const latestAgent = agentRows[0];
  const modelCoverage = coverageCount(agentRows, "ai_model");
  const providerCoverage = coverageCount(agentRows, "ai_provider");
  const configs = useMemo(() => integrationConfigs(apiURL, displayKey), [apiURL, displayKey]);
  const selectedConfig = configs.find((config) => config.id === selectedIntegration) ?? configs[0];
  const createIntegrationKey = useMutation({
    mutationFn: () => createKey("Integrations page", ["write_heartbeats", "read_stats", "read_summaries"]),
    onSuccess: (result) => {
      setLatestKey(result.data.api_key);
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
    }
  });
  useEffect(() => () => {
    if (copyTimer.current) {
      clearTimeout(copyTimer.current);
    }
  }, []);
  const copyText = async (id: string, text: string) => {
    await navigator.clipboard?.writeText(text);
    setCopied(id);
    if (copyTimer.current) {
      clearTimeout(copyTimer.current);
    }
    copyTimer.current = setTimeout(() => setCopied((current) => (current === id ? "" : current)), 1600);
  };

  return (
    <div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<PlugZap size={14} />}
        caption="Stint integrations"
        title="Integrations"
        sub="Connect your editor and agents to Stint, then enrich activity with model, provider, token, and cost-aware AI telemetry."
        actions={
          <>
            <button
              className="inline-flex w-fit items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-semibold text-ink hover:bg-sky-300 disabled:cursor-not-allowed disabled:opacity-60"
              type="button"
              onClick={() => createIntegrationKey.mutate()}
              disabled={createIntegrationKey.isPending}
            >
              <Plus size={16} /> {createIntegrationKey.isPending ? "Creating..." : "Create integration key"}
            </button>
            <Link className="inline-flex w-fit items-center gap-2 rounded-md border border-line bg-panel px-4 py-2 text-sm text-zinc-100 hover:border-accent/50 hover:bg-white/5" href="/settings">
              <KeyRound size={16} /> Manage keys <ArrowRight size={15} />
            </Link>
          </>
        }
      />
      {latestKey ? (
        <div className="mb-8 -mt-2 rounded border border-accent/35 bg-accent/10 p-4">
          <div className="mb-2 flex items-center gap-2 text-sm font-medium text-accent">
            <CheckCircle2 size={16} /> New key created
          </div>
          <div className="grid gap-3 lg:grid-cols-[1fr_auto] lg:items-center">
            <code className="overflow-x-auto rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-200">{latestKey}</code>
            <CopyButton id="latest-key" label="Copy key" copied={copied === "latest-key"} onCopy={() => copyText("latest-key", latestKey)} />
          </div>
        </div>
      ) : null}

      <section className="mb-6 grid gap-4 lg:grid-cols-3">
        <StatusTile icon={Cable} label="Endpoint" value={apiURL} />
        <StatusTile icon={KeyRound} label="API keys" value={keyCount > 0 ? `${keyCount} configured` : "Create one in Settings"} />
        <StatusTile icon={Code2} label="Known editors" value={editorCount > 0 ? `${editorCount} registry entries` : "Registry loading"} />
      </section>

      <section className="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
        <div className="space-y-6">
          <Panel title="Editor clients" icon={TerminalSquare} action={<Link className="text-sm text-accent hover:text-sky-300" href="/settings">Create key</Link>}>
            <div className="grid gap-3 md:grid-cols-2">
              {clients.map((client) => (
                <ClientCard
                  key={client.name}
                  {...client}
                  selected={selectedIntegration === client.recipeId}
                  onSelect={() => setSelectedIntegration(client.recipeId)}
                />
              ))}
            </div>
          </Panel>

          <Panel title={`${selectedConfig.name} instructions`} icon={Clipboard}>
            <IntegrationRecipe
              config={selectedConfig}
              copied={copied === selectedConfig.id}
              onCopy={() => copyText(selectedConfig.id, selectedConfig.lines.join("\n"))}
            />
          </Panel>

          <Panel title="Extended AI telemetry" icon={Bot}>
            <div className="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
              <div>
                <p className="text-sm leading-6 text-zinc-400">
                  Existing editor check-ins remain valid. Stint-native clients can add model-aware fields so dashboards can split GPT, Claude, Gemini, Codex, and future agent sessions without guessing from User-Agent strings.
                </p>
                <div className="mt-4 grid gap-2 text-sm">
                  {["ai_model or model_name", "llm_model", "ai_provider or llm_provider", "ai_agent and ai_agent_version", "ai_input_tokens and ai_output_tokens", "metadata for client-specific context"].map((item) => (
                    <div key={item} className="flex items-center gap-2 text-zinc-300">
                      <CheckCircle2 size={15} className="shrink-0 text-accent" /> {item}
                    </div>
                  ))}
                </div>
              </div>
              <CodeBlock
                lines={[
                  "{",
                  '  "entity": "/repo/app/page.tsx",',
                  '  "type": "file",',
                  '  "time": 1781887600,',
                  '  "project": "stint",',
                  '  "ai_model": "gpt-5.5-codex",',
                  '  "llm_model": "gpt-5.5-codex",',
                  '  "ai_provider": "openai",',
                  '  "ai_agent": "codex",',
                  '  "ai_input_tokens": 1200,',
                  '  "metadata": { "session_id": "..." }',
                  "}"
                ]}
              />
            </div>
          </Panel>
        </div>

        <div className="space-y-6">
          <Panel title="Connection health" icon={Radar}>
            <div className="mb-4 grid gap-3 sm:grid-cols-3">
              <HealthMetric label="Clients seen" value={String(agentRows.length)} />
              <HealthMetric label="Model coverage" value={`${modelCoverage}/${agentRows.length}`} />
              <HealthMetric label="Provider coverage" value={`${providerCoverage}/${agentRows.length}`} />
            </div>
            {latestAgent ? (
              <div className="rounded border border-line bg-ink">
                <div className="grid gap-3 border-b border-line px-4 py-3 text-xs uppercase tracking-[0.16em] text-zinc-500 md:grid-cols-[1fr_1fr_1fr]">
                  <span>Client</span>
                  <span>Telemetry</span>
                  <span>Last seen</span>
                </div>
                {agentRows.slice(0, 5).map((agent) => (
                  <div key={agent.id} className="grid gap-3 border-b border-line px-4 py-3 last:border-b-0 md:grid-cols-[1fr_1fr_1fr]">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-zinc-100">{agent.editor || "Unknown editor"}</div>
                      <div className="mt-1 truncate text-xs text-zinc-500">{agent.value}</div>
                    </div>
                    <div className="flex flex-wrap gap-2 text-xs">
                      <TelemetryPill label="model" value={agent.ai_model} />
                      <TelemetryPill label="provider" value={agent.ai_provider} />
                      <TelemetryPill label="agent" value={agent.ai_agent} />
                    </div>
                    <div className="text-sm text-zinc-400">{formatLastSeen(agent.last_seen_at)}</div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="rounded border border-dashed border-line bg-ink p-4 text-sm text-zinc-500">
                No clients have checked in yet. Send a heartbeat, then this panel will show last_seen_at, editor, model, and provider coverage.
              </div>
            )}
          </Panel>

          <Panel title="Existing editor clients" icon={ShieldCheck}>
            <p className="mb-4 text-sm leading-6 text-zinc-400">
              Use the clients you already have by pointing them at Stint. Basic auth, Bearer auth, and query-string API keys are accepted for compatibility.
            </p>
            <CopyableCodeBlock
              id="stock-wakatime-config"
              copied={copied === "stock-wakatime-config"}
              onCopy={() =>
                copyText(
                  "stock-wakatime-config",
                  ["[settings]", `api_url = ${apiURL}`, `api_key = ${displayKey}`, "heartbeat_rate_limit_seconds = 30"].join("\n")
                )
              }
              lines={[
                "[settings]",
                `api_url = ${apiURL}`,
                `api_key = ${displayKey}`,
                "heartbeat_rate_limit_seconds = 30"
              ]}
            />
            <div className="mt-4 rounded border border-line bg-ink p-3 text-xs leading-5 text-zinc-500">
              For multi-endpoint configs, use <span className="text-zinc-300">[api_urls]</span> and add <span className="text-zinc-300">.* = {apiURL}|waka_your_stint_key</span>. The native CLI also reads <span className="text-zinc-300">[DEFAULT]</span> top-level values, keeps runtime files directly under <span className="text-zinc-300">$WAKATIME_HOME/</span>, validates and normalizes <span className="text-zinc-300">api_url</span>, treats <span className="text-zinc-300">timeout = 0</span> as no HTTP timeout, treats non-integer <span className="text-zinc-300">heartbeat_rate_limit_seconds</span> config as the WakaTime default, treats negative <span className="text-zinc-300">heartbeat_rate_limit_seconds</span> as disabled, <span className="text-zinc-300">projectmap</span> with capture placeholders, <span className="text-zinc-300">.wakatime-project</span> <span className="text-zinc-300">{"{project}"}</span> interpolation from Git remotes, Git submodule maps, <span className="text-zinc-300">project_api_key</span>, <span className="text-zinc-300">api_key_vault_cmd</span>, WakaTime legacy aliases like <span className="text-zinc-300">apikey</span>, <span className="text-zinc-300">--api-key</span>, and <span className="text-zinc-300">settings.ignore</span>, case-insensitive scalar setting keys, quoted scalar values, ordered first-match project and API-key maps, all-match ordered api_urls fanout, and regex config, comma-separated include/exclude flags, newline-separated regex lists with commas preserved inside patterns, Perl-style lookahead regex fallback, HTTP/HTTPS/SOCKS/NTLM proxy settings, SSL options, machine/timezone request headers, dependency detection with WakaTime order and caps using the resolved or explicit language, WakaTime language aliases like <span className="text-zinc-300">CSharp</span>, <span className="text-zinc-300">CPP</span>, <span className="text-zinc-300">ObjectiveC</span>, and <span className="text-zinc-300">Visual Basic .NET</span>, and including side-effect and multi-line JavaScript/TypeScript imports, Scala grouped imports, multiline HTML script sources, Rust <span className="text-zinc-300">extern crate</span> declarations, and <span className="text-zinc-300">Gruntfile</span> as <span className="text-zinc-300">grunt</span>, WakaTime-style 5 MiB automatic local file-stat reads for language, dependency, and line metadata with unsaved file entities skipping automatic line and dependency detection, Vim modelines for <span className="text-zinc-300">--guess-language</span>, C-family, Objective-C, Matlab, Delphi, and F#/Forth language disambiguation, WakaTime top-language aliases such as <span className="text-zinc-300">crontab</span>, <span className="text-zinc-300">.ruby-version</span>, <span className="text-zinc-300">.Rprofile</span>, <span className="text-zinc-300">.sublime-settings</span>, <span className="text-zinc-300">.vue</span>, <span className="text-zinc-300">.svh</span>, <span className="text-zinc-300">.xaml</span>, <span className="text-zinc-300">.xpl</span>, <span className="text-zinc-300">.inc</span>, <span className="text-zinc-300">.i</span>, <span className="text-zinc-300">.j</span>, <span className="text-zinc-300">.mo</span>, <span className="text-zinc-300">.re</span>, <span className="text-zinc-300">.swg</span>, and <span className="text-zinc-300">.vm</span>, dependency hiding, <span className="text-zinc-300">hide_project_names</span> .wakatime-project aliases, <span className="text-zinc-300">hide_project_folder</span> filename fallback, automatic SSH/SFTP remote stats with credential stripping and WakaTime-style remote filter skipping, <span className="text-zinc-300">--local-file</span> overrides, and SSH config options such as <span className="text-zinc-300">HostName</span>, <span className="text-zinc-300">User</span>, <span className="text-zinc-300">Port</span>, <span className="text-zinc-300">IdentityFile</span>, <span className="text-zinc-300">UserKnownHostsFile</span>, <span className="text-zinc-300">HostKeyAlias</span>, and <span className="text-zinc-300">StrictHostKeyChecking</span>.
            </div>
          </Panel>

        </div>
      </section>
    </div>
  );
}

const clients = [
  {
    recipeId: "stint-cli-config",
    name: "Stint CLI",
    status: "live",
    description: "Use the native CLI for WakaTime-style heartbeats, status checks, projects, goals, file experts, and offline sync.",
    bullets: ["Drop-in root flags", "Git/Hg/SVN detection", "BoltDB offline queue"]
  },
  {
    recipeId: "wakatime-cli-config",
    name: "WakaTime CLI",
    status: "supported",
    description: "Use the official CLI and existing editor plugins by changing the API URL and key.",
    bullets: ["Heartbeat ingestion", "Durations and summaries", "User-Agent editor parsing"]
  },
  {
    recipeId: "codex-config",
    name: "Codex",
    status: "supported",
    description: "Codex heartbeats are parsed for editor, OS, agent, and version labels when available.",
    bullets: ["Agent attribution", "Token cost metrics", "Project and branch detection"]
  },
  {
    recipeId: "vscode-config",
    name: "VS Code",
    status: "compatible",
    description: "Use the existing extension, then point the shared config file at Stint.",
    bullets: ["Extension marketplace", "Project/language charts", "Machine and OS breakdowns"]
  },
  {
    recipeId: "jetbrains-config",
    name: "JetBrains",
    status: "compatible",
    description: "JetBrains IDEs work through the existing plugin and the same config file.",
    bullets: ["Basic and Bearer auth", "Project/language charts", "Machine and OS breakdowns"]
  },
  {
    recipeId: "vim-config",
    name: "Vim/Neovim",
    status: "compatible",
    description: "Terminal editors can send standard activity check-ins to Stint.",
    bullets: ["Shared ~/.wakatime.cfg", "Project .wakatime overrides", "Branch and language inference"]
  },
  {
    recipeId: "shell-cli-config",
    name: "Shell CLI",
    status: "compatible",
    description: "Use curl or any HTTP client to send WakaTime-shaped heartbeats directly.",
    bullets: ["Bearer auth", "JSON payloads", "Smoke-test friendly"]
  }
] as const;

function integrationConfigs(apiURL: string, apiKey: string) {
  return [
    {
      id: "stint-cli-config",
      name: "Stint CLI",
      description: "Build the native client, initialize config, then send a compatibility heartbeat.",
      lines: [
        "make stint",
        `bin/stint config init --api-url ${apiURL} --api-key ${apiKey}`,
        "bin/stint api-keys",
        'bin/stint api-keys create "Editor key" --scope write_heartbeats --scope read_stats',
        "bin/stint api-keys delete API_KEY_ID",
        "bin/stint oauth-apps",
        'bin/stint oauth-apps create "Local OAuth app" --redirect-uri http://localhost:3000/callback --scope read_stats',
        "bin/stint oauth-apps delete OAUTH_APP_ID",
        "bin/stint oauth token --client-id OAUTH_CLIENT_ID --client-secret OAUTH_CLIENT_SECRET --code AUTH_CODE --redirect-uri http://localhost:3000/callback",
        "bin/stint oauth token --client-id OAUTH_CLIENT_ID --client-secret OAUTH_CLIENT_SECRET --refresh-token REFRESH_TOKEN",
        "bin/stint oauth revoke ACCESS_OR_REFRESH_TOKEN --client-id OAUTH_CLIENT_ID --client-secret OAUTH_CLIENT_SECRET",
        "bin/stint account",
        "bin/stint account update account.json",
        "bin/stint account delete --confirm",
        "bin/stint meta",
        "bin/stint api-docs",
        "bin/stint leaders",
        "bin/stint leaders --language Go --country US",
        "bin/stint editors",
        "bin/stint program-languages",
        "bin/stint users public-username",
        "bin/stint users public-username stats last_7_days",
        "bin/stint users public-username stats --range last_30_days",
        "bin/stint users public-username summaries",
        "bin/stint users public-username summaries --start 2026-06-01 --end 2026-06-30",
        "bin/stint share SHARE_TOKEN stats",
        "bin/stint share SHARE_TOKEN stats --range last_7_days",
        "bin/stint share SHARE_TOKEN summaries",
        "bin/stint share SHARE_TOKEN summaries --start 2026-06-01 --end 2026-06-30",
        "bin/stint health",
        "bin/stint health ingestion",
        "bin/stint dev seed-key --github-id 4001 --username local-dev",
        "bin/stint dev heartbeats-purge --retention-days 0",
        "bin/stint dev leaderboard-update --range last_7_days",
        "bin/stint dev goals-evaluate",
        'bin/stint heartbeat --entity "$PWD/main.go" --write --project stint',
        'bin/stint heartbeat --entity "$PWD/main.go" --category "ai coding" --ai-model gpt-5-codex --ai-provider openai --ai-agent codex --metadata \'{"source":"manual"}\'',
        'bin/stint heartbeats "$(date +%F)"',
        "bin/stint today",
        "bin/stint today --output json",
        "bin/stint today-goal 00000000-0000-4000-8000-000000000000",
        "bin/stint stats last_7_days",
        'bin/stint durations "$(date +%F)" --slice-by language',
        'bin/stint summaries 2026-06-01 2026-06-30',
        "bin/stint projects stint",
        "bin/stint projects stint commits --branch main",
        "bin/stint projects stint commits COMMIT_HASH",
        "bin/stint all-time",
        "bin/stint machine-names",
        "bin/stint user-agents",
        "bin/stint goals",
        "bin/stint goals create goal.json",
        "bin/stint goals update GOAL_ID goal.json",
        "bin/stint goals delete GOAL_ID",
        "bin/stint insights languages last_7_days",
        "bin/stint external-durations",
        "bin/stint external-durations create external-duration.json",
        "bin/stint external-durations bulk external-durations.json",
        "bin/stint external-durations delete 00000000-0000-4000-8000-000000000000",
        "bin/stint external-durations delete --ids id-1,id-2",
        "bin/stint custom-pricing",
        "bin/stint custom-pricing upsert custom-pricing.json",
        "bin/stint custom-pricing delete gpt-5-codex",
        "bin/stint pricing-sources",
        "bin/stint pricing-models",
        "bin/stint billing-prefs",
        "bin/stint billing-prefs upsert billing-pref.json",
        "bin/stint billing-prefs delete codex",
        "bin/stint ai-costs",
        "bin/stint ai-costs replace ai-costs.json",
        "bin/stint leaderboards",
        "bin/stint leaderboards create leaderboard.json",
        "bin/stint leaderboards update BOARD_ID leaderboard.json",
        "bin/stint leaderboards add-member BOARD_ID github-username",
        "bin/stint leaderboards remove-member BOARD_ID USER_ID",
        "bin/stint leaderboards delete BOARD_ID",
        "bin/stint share-tokens",
        'bin/stint share-tokens create "Public read"',
        "bin/stint share-tokens delete SHARE_TOKEN_ID",
        "bin/stint events",
        'bin/stint usage-events --start "$(date +%F)" --end "$(date +%F)"',
        "bin/stint usage-events summary --range last_30_days --cost-mode calculate",
        "bin/stint usage-events blocks --range last_7_days",
        "bin/stint data-dumps create heartbeats",
        "bin/stint data-dumps create daily",
        "bin/stint data-dumps",
        "bin/stint data-dumps download DUMP_ID",
        "bin/stint custom-rules",
        "bin/stint custom-rules replace custom-rules.json",
        "bin/stint custom-rules delete RULE_ID",
        "bin/stint custom-rules progress",
        "bin/stint custom-rules abort",
        "bin/stint import wakatime ~/Downloads/wakatime-dump.json",
        "gzip -dc ~/Downloads/wakatime-dump.json.gz | bin/stint import wakatime --stdin",
        'bin/stint file-experts "$PWD/main.go"',
        "bin/stint doctor",
        "bin/stint collect",
        'bin/stint --file-experts --entity "$PWD/main.go"',
        "bin/stint --offline-count",
        "bin/stint --print-offline-heartbeats 10",
        "bin/stint --sync-ai-activity",
        "bin/stint --sync-ai-heartbeats",
        "bin/stint offline count",
        "bin/stint offline print",
        "bin/stint offline sync"
      ]
    },
    {
      id: "wakatime-cli-config",
      name: "WakaTime CLI",
      description: "Install or keep the upstream CLI, then point its shared WakaTime config at Stint.",
      lines: [
        "pipx install wakatime",
        "mkdir -p ~/.wakatime",
        "cat > ~/.wakatime.cfg <<'EOF'",
        "[settings]",
        `api_url = ${apiURL}`,
        `api_key = ${apiKey}`,
        "heartbeat_rate_limit_seconds = 30",
        "offline = true",
        "EOF",
        'wakatime-cli --entity "$PWD/main.go" --write --plugin shell/1.0.0',
        "wakatime-cli --today",
        "wakatime-cli --offline-count",
        "wakatime-cli --sync-offline-activity"
      ]
    },
    {
      id: "codex-config",
      name: "Codex",
      description: "Use the native Stint CLI to sync local Codex sessions and emit model-aware AI activity.",
      lines: [
        "make stint",
        `bin/stint config init --api-url ${apiURL} --api-key ${apiKey}`,
        "bin/stint --sync-ai-activity --agent codex",
        "bin/stint --sync-ai-heartbeats --agent codex",
        'bin/stint heartbeat --entity "$PWD/main.go" --category "ai coding" --ai-agent codex --ai-provider openai --ai-model gpt-5-codex --write',
        "bin/stint today --output json",
        "bin/stint user-agents"
      ]
    },
    {
      id: "vscode-config",
      name: "VS Code",
      description: "Install the editor extension, then use this shared config.",
      lines: [
        "Install the WakaTime extension from the VS Code Marketplace.",
        "mkdir -p ~/.wakatime",
        "cat > ~/.wakatime.cfg <<'EOF'",
        "[settings]",
        `api_url = ${apiURL}`,
        `api_key = ${apiKey}`,
        "import_cfg = ~/.wakatime/private.cfg",
        "heartbeat_rate_limit_seconds = 30",
        "status_bar_enabled = true",
        "status_bar_show_categories = true",
        "status_bar_coding_activity = true",
        "EOF",
        "Reload VS Code, edit a file, then check Stint > Integrations for the latest user agent."
      ]
    },
    {
      id: "jetbrains-config",
      name: "JetBrains",
      description: "Install the activity plugin from JetBrains Marketplace and reuse the same API URL.",
      lines: [
        "Install the WakaTime plugin from JetBrains Marketplace.",
        "mkdir -p ~/.wakatime",
        "cat > ~/.wakatime.cfg <<'EOF'",
        "[settings]",
        `api_url = ${apiURL}`,
        `api_key = ${apiKey}`,
        "heartbeat_rate_limit_seconds = 30",
        "hide_project_names = false",
        "EOF",
        "Restart the IDE, open a project, edit a file, then check Stint > Integrations for the JetBrains user agent."
      ]
    },
    {
      id: "vim-config",
      name: "Vim/Neovim",
      description: "Keep a single config file for terminal editor plugins.",
      lines: [
        "Install vim-wakatime for Vim or Neovim.",
        "mkdir -p ~/.wakatime",
        "cat > ~/.wakatime.cfg <<'EOF'",
        "[settings]",
        `api_url = ${apiURL}`,
        `api_key = ${apiKey}`,
        "heartbeat_rate_limit_seconds = 30",
        "debug = false",
        "EOF",
        "Open Vim or Neovim, edit a tracked file, then run :WakaTimeApiKey if the plugin asks for a key."
      ]
    },
    {
      id: "shell-cli-config",
      name: "Shell CLI",
      description: "Send a compatibility heartbeat directly for smoke testing.",
      lines: [
        `curl -X POST ${apiURL}/users/current/heartbeats \\`,
        `  -H "Authorization: Bearer ${apiKey}" \\`,
        '  -H "Content-Type: application/json" \\',
        '  -d \'{"entity":"~/src/stint/main.go","type":"file","time":1781887600,"project":"stint","language":"Go"}\''
      ]
    }
  ] as const;
}

function coverageCount(rows: UserAgent[], field: "ai_model" | "ai_provider") {
  return rows.filter((row) => Boolean(row[field]?.trim())).length;
}

function Panel({ title, icon: Icon, action, children }: { title: string; icon: typeof PlugZap; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="rounded border border-line bg-panel">
      <div className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-medium text-zinc-100">
          <Icon size={16} className="text-accent" /> {title}
        </div>
        {action}
      </div>
      <div className="p-4">{children}</div>
    </section>
  );
}

function StatusTile({ icon: Icon, label, value }: { icon: typeof PlugZap; label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-panel p-4">
      <div className="mb-3 flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
        <Icon size={14} className="text-accent" /> {label}
      </div>
      <div className="break-words text-sm font-medium text-zinc-100">{value}</div>
    </div>
  );
}

function HealthMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-ink p-3">
      <div className="text-[11px] uppercase tracking-[0.14em] text-zinc-500">{label}</div>
      <div className="mt-2 text-2xl font-semibold text-white">{value}</div>
    </div>
  );
}

function TelemetryPill({ label, value }: { label: string; value?: string }) {
  const present = Boolean(value?.trim());
  return (
    <span className={`rounded border px-2 py-1 ${present ? "border-accent/35 bg-accent/10 text-accent" : "border-line text-zinc-600"}`}>
      {label}: {present ? value : "missing"}
    </span>
  );
}

function ClientCard({ name, status, description, bullets, selected, onSelect }: { name: string; status: string; description: string; bullets: readonly string[]; selected: boolean; onSelect: () => void }) {
  return (
    <button
      className={`rounded border p-4 text-left transition hover:border-accent/60 hover:bg-white/5 focus:outline-none focus:ring-2 focus:ring-accent/60 ${selected ? "border-accent/60 bg-accent/10" : "border-line bg-ink"}`}
      type="button"
      onClick={onSelect}
      aria-label={`Show ${name} integration instructions`}
      aria-pressed={selected}
      aria-controls="integration-instructions"
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="font-medium text-zinc-100">{name}</h3>
        <span className="rounded border border-line px-2 py-1 text-[11px] uppercase tracking-[0.14em] text-zinc-400">{status}</span>
      </div>
      <p className="mt-3 min-h-[60px] text-sm leading-5 text-zinc-500">{description}</p>
      <div className="mt-4 space-y-2">
        {bullets.map((bullet) => (
          <div key={bullet} className="flex items-center gap-2 text-sm text-zinc-300">
            <CheckCircle2 size={14} className="shrink-0 text-accent" /> {bullet}
          </div>
        ))}
      </div>
    </button>
  );
}

function IntegrationRecipe({ config, copied, onCopy }: { config: ReturnType<typeof integrationConfigs>[number]; copied: boolean; onCopy: () => void }) {
  return (
    <div id="integration-instructions" className="rounded border border-line bg-ink p-4">
      <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="font-medium text-zinc-100">{config.name}</h3>
          <p className="mt-1 text-sm leading-5 text-zinc-500">{config.description}</p>
        </div>
        <CopyButton id={config.id} label="Copy config" copied={copied} onCopy={onCopy} />
      </div>
      <CodeBlock lines={config.lines} />
    </div>
  );
}

function CopyableCodeBlock({ id, lines, copied, onCopy }: { id: string; lines: readonly string[]; copied: boolean; onCopy: () => void }) {
  return (
    <div>
      <div className="mb-2 flex justify-end">
        <CopyButton id={id} label="Copy config" copied={copied} onCopy={onCopy} />
      </div>
      <CodeBlock lines={lines} />
    </div>
  );
}

function CopyButton({ id, label, copied, onCopy }: { id: string; label: string; copied: boolean; onCopy: () => void }) {
  return (
    <button
      className="inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded border border-line px-3 text-sm text-zinc-200 hover:border-accent/50 hover:bg-white/5"
      type="button"
      aria-label={label}
      data-copy-id={id}
      onClick={onCopy}
    >
      {copied ? <Check size={15} className="text-accent" /> : <Clipboard size={15} />}
      {copied ? "Copied" : label}
    </button>
  );
}

function formatLastSeen(value?: string) {
  if (!value) {
    return "No last_seen_at";
  }
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}

function CodeBlock({ lines }: { lines: readonly string[] }) {
  return (
    <pre className="overflow-x-auto rounded border border-line bg-[#070b10] p-4 text-xs leading-6 text-zinc-300">
      <code>{lines.join("\n")}</code>
    </pre>
  );
}
