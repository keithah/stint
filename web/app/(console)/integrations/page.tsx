"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import {
  ArrowRight,
  Check,
  Clipboard,
  ExternalLink,
  KeyRound,
  PlugZap,
  RefreshCw,
  TerminalSquare,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "@/components/ui";
import {
  createKey,
  listKeys,
  listUserAgents,
  serverMeta,
  type UserAgent,
} from "@/lib/api";
import {
  clients,
  compatibilityNote,
  integrationConfigs,
  stintConfiguredInstallCommand,
  type IntegrationConfig,
} from "./recipes";

const catalogGroups = [
  { label: "Stint", names: ["Stint CLI", "Codex", "Claude Code"] },
  { label: "Editors", names: ["VS Code", "JetBrains", "Vim/Neovim"] },
  { label: "Compatibility", names: ["WakaTime CLI", "Shell CLI"] },
] as const;

export default function IntegrationsPage() {
  return <IntegrationsContent />;
}

function IntegrationsContent() {
  const queryClient = useQueryClient();
  const [latestKey, setLatestKey] = useState("");
  const [latestKeyId, setLatestKeyId] = useState("");
  const [copied, setCopied] = useState("");
  const [setupMessage, setSetupMessage] = useState("");
  const [validateMessage, setValidateMessage] = useState("");
  const [selectedIntegration, setSelectedIntegration] =
    useState("stint-cli-config");
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const meta = useQuery({
    queryKey: ["server-meta"],
    queryFn: serverMeta,
    staleTime: 60000,
  });
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: listKeys });
  const userAgents = useQuery({
    queryKey: ["user-agents"],
    queryFn: listUserAgents,
    staleTime: 60000,
  });
  const apiURL = meta.data?.data.api_url || "https://stint.fyi/api/v1";
  const displayKey = latestKey || "stint_your_stint_key";
  const agentRows = userAgents.data?.data ?? [];
  const recentStintAgent = agentRows.find((agent) => isStintAgent(agent));
  const stintCLIConnected = Boolean(recentStintAgent);
  const configs = useMemo(
    () => integrationConfigs(apiURL, displayKey),
    [apiURL, displayKey],
  );
  const generatedSetupCommand = stintConfiguredInstallCommand(
    apiURL,
    displayKey,
  );
  const selectedConfig =
    configs.find((config) => config.id === selectedIntegration) ?? configs[0];
  const groupedClients = catalogGroups.map((group) => ({
    ...group,
    clients: clients.filter((client) =>
      (group.names as readonly string[]).includes(client.name),
    ),
  }));
  const createIntegrationKey = useMutation({
    mutationFn: () =>
      createKey("Integrations page", [
        "write_heartbeats",
        "read_stats",
        "read_summaries",
      ]),
    onSuccess: (result) => {
      setLatestKey(result.data.api_key);
      setLatestKeyId(result.data.key.id);
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
    },
  });
  useEffect(
    () => () => {
      if (copyTimer.current) {
        clearTimeout(copyTimer.current);
      }
    },
    [],
  );
  useEffect(() => {
    const selectFromHash = () => {
      const hash = window.location.hash.slice(1);
      if (configs.some((config) => config.id === hash)) {
        setSelectedIntegration(hash);
      }
    };
    selectFromHash();
    window.addEventListener("hashchange", selectFromHash);
    return () => window.removeEventListener("hashchange", selectFromHash);
  }, [configs]);
  const copyText = async (id: string, text: string) => {
    await navigator.clipboard?.writeText(text);
    setCopied(id);
    if (copyTimer.current) {
      clearTimeout(copyTimer.current);
    }
    copyTimer.current = setTimeout(
      () => setCopied((current) => (current === id ? "" : current)),
      1600,
    );
  };
  const copyGeneratedSetup = async () => {
    setSetupMessage("");
    setValidateMessage("");
    const result = latestKey
      ? { data: { api_key: latestKey, key: { id: latestKeyId } } }
      : await createIntegrationKey.mutateAsync();
    const apiKey = result.data.api_key;
    const apiKeyId = result.data.key.id;
    setLatestKey(apiKey);
    setLatestKeyId(apiKeyId);
    setSelectedIntegration("stint-cli-config");
    window.history.replaceState(null, "", "#stint-cli-config");
    await copyText(
      "generated-setup",
      stintConfiguredInstallCommand(apiURL, apiKey),
    );
    setSetupMessage("Setup command copied with your Stint key.");
  };
  const validateConnection = async () => {
    setValidateMessage("Checking for a Stint CLI check-in...");
    const [agentsResult, keysResult] = await Promise.all([
      userAgents.refetch(),
      keys.refetch(),
    ]);
    const generatedKeyUsed = Boolean(
      latestKeyId &&
        keysResult.data?.data.some(
          (key) => key.id === latestKeyId && key.last_used_at,
        ),
    );
    const connected =
      (agentsResult.data?.data ?? []).some((agent) => isStintAgent(agent)) ||
      generatedKeyUsed;
    setValidateMessage(
      connected
        ? "Yes, Stint CLI is connected."
        : "No Stint CLI check-in yet. Run the copied command, then verify again.",
    );
  };

  return (
    <div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<PlugZap size={14} />}
        caption="Stint integrations"
        title="Integrations"
        sub="Connect Stint to your editors and AI coding agents."
        actions={
          <Link
            className="inline-flex w-fit items-center gap-2 rounded-md border border-line bg-panel px-4 py-2 text-sm text-zinc-100 hover:border-accent/50 hover:bg-white/5"
            href="/settings"
          >
            <KeyRound size={16} /> Manage keys <ArrowRight size={15} />
          </Link>
        }
      />

      <section className="mb-8 rounded border border-accent/35 bg-accent/10 p-4">
        <div className="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-start">
          <div className="min-w-0">
            <div className="mb-2 flex items-center gap-2 text-sm font-medium text-accent">
              <TerminalSquare size={16} /> Set up Stint CLI
            </div>
            <p className="mb-3 max-w-2xl text-sm leading-6 text-zinc-300">
              One command creates your key, installs the CLI, writes
              configuration, and runs doctor.
            </p>
            <code className="block overflow-x-auto rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-200">
              {generatedSetupCommand}
            </code>
            <p className="mt-3 text-sm text-zinc-400">
              {validateMessage ||
                setupMessage ||
                (stintCLIConnected
                  ? `Yes, Stint CLI is connected${recentStintAgent?.last_seen_at ? ` · ${formatLastSeen(recentStintAgent.last_seen_at)}` : ""}.`
                  : "Copy setup, run it once, then verify the connection.")}
            </p>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row lg:flex-col">
            <button
              className="inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-ink hover:bg-sky-300 disabled:cursor-not-allowed disabled:opacity-60"
              type="button"
              onClick={() => {
                void copyGeneratedSetup();
              }}
              disabled={createIntegrationKey.isPending}
            >
              {copied === "generated-setup" ? (
                <Check size={15} />
              ) : (
                <Clipboard size={15} />
              )}
              {createIntegrationKey.isPending
                ? "Creating..."
                : copied === "generated-setup"
                  ? "Copied"
                  : "Copy setup"}
            </button>
            <button
              className="inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-md border border-line px-3 text-sm text-zinc-200 hover:border-accent/50 hover:bg-white/5 disabled:opacity-60"
              type="button"
              onClick={() => {
                void validateConnection();
              }}
              disabled={userAgents.isFetching || keys.isFetching}
            >
              <RefreshCw
                size={15}
                className={
                  userAgents.isFetching || keys.isFetching ? "animate-spin" : ""
                }
              />
              Verify connection
            </button>
          </div>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[1fr_360px]">
        <div className="min-w-0">
          <div className="mb-4 flex items-end justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold text-zinc-100">
                Integration catalog
              </h2>
              <p className="mt-1 text-sm text-zinc-500">
                Pick the client you use. Most setup details stay out of the way
                until you need them.
              </p>
            </div>
          </div>

          <div className="space-y-6">
            {groupedClients.map((group) => (
              <div key={group.label}>
                <div className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-zinc-500">
                  {group.label}
                </div>
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {group.clients.map((client) => (
                    <ClientCard
                      key={client.name}
                      {...client}
                      selected={selectedIntegration === client.recipeId}
                      onSelect={(recipeId) => {
                        setSelectedIntegration(recipeId);
                        window.history.replaceState(null, "", `#${recipeId}`);
                      }}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        <DetailPanel
          config={selectedConfig}
          copied={copied === selectedConfig.id}
          onCopy={() =>
            copyText(selectedConfig.id, recipeCopyText(selectedConfig))
          }
        />
      </section>
    </div>
  );
}

function isStintAgent(agent: UserAgent) {
  const value = `${agent.editor ?? ""} ${agent.value ?? ""}`.toLowerCase();
  return value.includes("stint");
}

function ClientCard({
  recipeId,
  name,
  status,
  description,
  selected,
  onSelect,
}: {
  recipeId: string;
  name: string;
  status: string;
  description: string;
  selected: boolean;
  onSelect: (recipeId: string) => void;
}) {
  const href = `#${recipeId}`;
  return (
    <a
      className={`rounded border p-4 text-left transition hover:border-accent/60 hover:bg-white/5 focus:outline-none focus:ring-2 focus:ring-accent/60 ${selected ? "border-accent/60 bg-accent/10" : "border-line bg-panel"}`}
      href={href}
      role="button"
      onClick={() => onSelect(recipeId)}
      aria-label={`Show ${name} integration instructions`}
      aria-pressed={selected}
      aria-controls="integration-instructions"
    >
      <div className="mb-3 flex items-start justify-between gap-3">
        <h3 className="font-medium text-zinc-100">{name}</h3>
        <span className="rounded border border-line px-2 py-1 text-[11px] uppercase tracking-[0.14em] text-zinc-400">
          {status}
        </span>
      </div>
      <p className="text-sm leading-5 text-zinc-500">{description}</p>
    </a>
  );
}

function DetailPanel({
  config,
  copied,
  onCopy,
}: {
  config: IntegrationConfig;
  copied: boolean;
  onCopy: () => void;
}) {
  return (
    <aside
      id="integration-instructions"
      className="min-w-0 rounded border border-line bg-panel"
      aria-label="integration-instructions"
    >
      <span id={config.id} className="sr-only">
        {config.name}
      </span>
      <div className="border-b border-line p-4">
        <div className="mb-3 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="font-medium text-zinc-100">{config.name}</h2>
            <p className="mt-1 text-sm leading-5 text-zinc-500">
              {config.description}
            </p>
          </div>
          <CopyButton
            id={config.id}
            label="Copy"
            copied={copied}
            onCopy={onCopy}
          />
        </div>
        <p className="text-xs leading-5 text-zinc-500">{compatibilityNote}</p>
      </div>
      <div className="space-y-3 p-4">
        {config.options.map((option) => (
          <SetupOptionCard key={option.title} option={option} />
        ))}
        {config.notes?.length ? (
          <div className="rounded border border-line bg-ink p-3 text-xs leading-5 text-zinc-500">
            {config.notes.map((note) => (
              <p key={note} className="mb-2 last:mb-0">
                {note}
              </p>
            ))}
          </div>
        ) : null}
      </div>
    </aside>
  );
}

function SetupOptionCard({
  option,
}: {
  option: IntegrationConfig["options"][number];
}) {
  return (
    <div className="rounded border border-line bg-ink p-3">
      <div className="mb-2 flex items-start justify-between gap-3">
        <h3 className="text-sm font-medium text-zinc-100">{option.title}</h3>
        <span className="shrink-0 rounded border border-line px-2 py-1 text-[11px] uppercase tracking-[0.12em] text-zinc-500">
          {option.badge}
        </span>
      </div>
      <p className="text-sm leading-5 text-zinc-500">{option.description}</p>
      {option.commands?.length ? (
        <div className="mt-3">
          <CodeBlock lines={option.commands} />
        </div>
      ) : null}
      {option.link ? (
        <a
          className="mt-3 inline-flex items-center gap-2 text-sm text-accent hover:text-sky-300"
          href={option.link.href}
        >
          {option.link.label} <ExternalLink size={14} />
        </a>
      ) : null}
    </div>
  );
}

function recipeCopyText(config: IntegrationConfig) {
  const commands = config.options.flatMap((option) => option.commands ?? []);
  if (commands.length) {
    return [...new Set(commands)].join("\n");
  }
  return config.options
    .map((option) => option.link?.href)
    .filter(Boolean)
    .join("\n");
}

function CopyButton({
  id,
  label,
  copied,
  onCopy,
}: {
  id: string;
  label: string;
  copied: boolean;
  onCopy: () => void;
}) {
  return (
    <button
      className="inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded border border-line px-3 text-sm text-zinc-200 hover:border-accent/50 hover:bg-white/5"
      type="button"
      aria-label={label}
      data-copy-id={id}
      onClick={onCopy}
    >
      {copied ? (
        <Check size={15} className="text-accent" />
      ) : (
        <Clipboard size={15} />
      )}
      {copied ? "Copied" : label}
    </button>
  );
}

function formatLastSeen(value?: string) {
  if (!value) {
    return "No last_seen_at";
  }
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}

function CodeBlock({ lines }: { lines: readonly string[] }) {
  return (
    <pre className="overflow-x-auto rounded border border-line bg-[#070b10] p-3 text-xs leading-6 text-zinc-300">
      <code>{lines.join("\n")}</code>
    </pre>
  );
}
