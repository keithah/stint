"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { ArrowRight, Bot, Cable, Check, CheckCircle2, Clipboard, Code2, KeyRound, PlugZap, Plus, Radar, ShieldCheck, TerminalSquare } from "lucide-react";
import { useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { createKey, listEditors, listKeys, listUserAgents, serverMeta, wakatimeAPIURL, type UserAgent } from "@/lib/api";

export default function IntegrationsPage() {
  return (
    <AppShell>
      <IntegrationsContent />
    </AppShell>
  );
}

function IntegrationsContent() {
  const queryClient = useQueryClient();
  const [latestKey, setLatestKey] = useState("");
  const [copied, setCopied] = useState("");
  const meta = useQuery({ queryKey: ["server-meta"], queryFn: serverMeta, retry: false, staleTime: 60000 });
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: listKeys, retry: false });
  const editors = useQuery({ queryKey: ["editors"], queryFn: listEditors, retry: false, staleTime: 3600000 });
  const userAgents = useQuery({ queryKey: ["user-agents"], queryFn: listUserAgents, retry: false, staleTime: 60000 });
  const apiURL = meta.data?.data.api_url || wakatimeAPIURL() || "https://stint.fyi/api/v1";
  const displayKey = latestKey || "waka_your_stint_key";
  const keyCount = keys.data?.data.length ?? 0;
  const editorCount = editors.data?.data.length ?? 0;
  const agentRows = userAgents.data?.data ?? [];
  const latestAgent = agentRows[0];
  const modelCoverage = coverageCount(agentRows, "ai_model");
  const providerCoverage = coverageCount(agentRows, "ai_provider");
  const configs = useMemo(() => integrationConfigs(apiURL, displayKey), [apiURL, displayKey]);
  const createIntegrationKey = useMutation({
    mutationFn: () => createKey("Integrations page", ["write_heartbeats", "read_stats", "read_summaries"]),
    onSuccess: (result) => {
      setLatestKey(result.data.api_key);
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
    }
  });
  const copyText = async (id: string, text: string) => {
    await navigator.clipboard?.writeText(text);
    setCopied(id);
    window.setTimeout(() => setCopied((current) => (current === id ? "" : current)), 1600);
  };

  return (
    <div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
      <header className="mb-8 border-b border-line pb-6">
        <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
          <PlugZap size={14} /> Stint integrations
        </div>
        <div className="grid gap-5 lg:grid-cols-[1fr_auto] lg:items-end">
          <div>
            <h1 className="text-4xl font-semibold tracking-tight text-white">Integrations</h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-zinc-400">
              Connect your editor and agents to Stint, then enrich activity with model, provider, token, and cost-aware AI telemetry.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
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
          </div>
        </div>
        {latestKey ? (
          <div className="mt-5 rounded border border-accent/35 bg-accent/10 p-4">
            <div className="mb-2 flex items-center gap-2 text-sm font-medium text-accent">
              <CheckCircle2 size={16} /> New key created
            </div>
            <div className="grid gap-3 lg:grid-cols-[1fr_auto] lg:items-center">
              <code className="overflow-x-auto rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-200">{latestKey}</code>
              <CopyButton id="latest-key" label="Copy key" copied={copied === "latest-key"} onCopy={() => copyText("latest-key", latestKey)} />
            </div>
          </div>
        ) : null}
      </header>

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
                <ClientCard key={client.name} {...client} />
              ))}
            </div>
          </Panel>

          <Panel title="Setup recipes" icon={Clipboard}>
            <div className="grid gap-3">
              {configs.map((config) => (
                <IntegrationRecipe
                  key={config.name}
                  config={config}
                  copied={copied === config.id}
                  onCopy={() => copyText(config.id, config.lines.join("\n"))}
                />
              ))}
            </div>
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
              For multi-endpoint configs, use <span className="text-zinc-300">[api_urls]</span> and add <span className="text-zinc-300">.* = {apiURL}|waka_your_stint_key</span>.
            </div>
          </Panel>

          <Panel title="Stint client roadmap" icon={PlugZap}>
            <div className="space-y-3">
              {roadmap.map((item) => (
                <div key={item.title} className="rounded border border-line bg-ink p-4">
                  <div className="flex items-center justify-between gap-3">
                    <h3 className="font-medium text-zinc-100">{item.title}</h3>
                    <span className="rounded border border-accent/30 bg-accent/10 px-2 py-1 text-[11px] uppercase tracking-[0.14em] text-accent">{item.state}</span>
                  </div>
                  <p className="mt-2 text-sm leading-5 text-zinc-500">{item.description}</p>
                </div>
              ))}
            </div>
          </Panel>
        </div>
      </section>
    </div>
  );
}

const clients = [
  {
    name: "Stint CLI",
    status: "planned",
    description: "A native client with first-class model, provider, and token telemetry.",
    bullets: ["Existing payload parity", "Extended AI metadata", "Local agent adapters"]
  },
  {
    name: "WakaTime CLI",
    status: "supported",
    description: "Use the official CLI and existing editor plugins by changing the API URL and key.",
    bullets: ["Heartbeat ingestion", "Durations and summaries", "User-Agent editor parsing"]
  },
  {
    name: "Codex",
    status: "supported",
    description: "Codex heartbeats are parsed for editor, OS, agent, and version labels when available.",
    bullets: ["Agent attribution", "Token cost metrics", "Project and branch detection"]
  },
  {
    name: "VS Code",
    status: "compatible",
    description: "Use the existing extension, then point the shared config file at Stint.",
    bullets: ["Extension marketplace", "Project/language charts", "Machine and OS breakdowns"]
  },
  {
    name: "JetBrains",
    status: "compatible",
    description: "JetBrains IDEs work through the existing plugin and the same config file.",
    bullets: ["Basic and Bearer auth", "Project/language charts", "Machine and OS breakdowns"]
  },
  {
    name: "Vim/Neovim",
    status: "compatible",
    description: "Terminal editors can send standard activity check-ins to Stint.",
    bullets: ["Shared ~/.wakatime.cfg", "Shell CLI support", "Branch and language inference"]
  }
] as const;

function integrationConfigs(apiURL: string, apiKey: string) {
  return [
    {
      id: "vscode-config",
      name: "VS Code",
      description: "Install the editor extension, then use this shared config.",
      lines: ["[settings]", `api_url = ${apiURL}`, `api_key = ${apiKey}`, "heartbeat_rate_limit_seconds = 30"]
    },
    {
      id: "jetbrains-config",
      name: "JetBrains",
      description: "Install the activity plugin from JetBrains Marketplace and reuse the same API URL.",
      lines: ["[settings]", `api_url = ${apiURL}`, `api_key = ${apiKey}`, "hide_project_names = false"]
    },
    {
      id: "vim-config",
      name: "Vim/Neovim",
      description: "Keep a single config file for terminal editor plugins.",
      lines: ["[settings]", `api_url = ${apiURL}`, `api_key = ${apiKey}`, "debug = false"]
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

const roadmap = [
  { title: "Model-aware ingestion", state: "live", description: "Heartbeats can include ai_model, llm_model, ai_provider, token counts, and structured metadata." },
  { title: "Native Stint CLI", state: "next", description: "A forkable client can enrich standard editor payloads without breaking existing plugins." },
  { title: "Integration catalog", state: "next", description: "Per-editor setup cards, copyable configs, and validation checks will move here from Settings." }
] as const;

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

function ClientCard({ name, status, description, bullets }: { name: string; status: string; description: string; bullets: readonly string[] }) {
  return (
    <div className="rounded border border-line bg-ink p-4">
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
    </div>
  );
}

function IntegrationRecipe({ config, copied, onCopy }: { config: ReturnType<typeof integrationConfigs>[number]; copied: boolean; onCopy: () => void }) {
  return (
    <div className="rounded border border-line bg-ink p-4">
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
