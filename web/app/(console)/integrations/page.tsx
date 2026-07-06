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
  integrationConfigs,
  stintConfiguredInstallCommand,
  type IntegrationConfig,
} from "./recipes";

type ToolCategoryId = "terminal" | "agents" | "editors";

const toolCategories: {
  id: ToolCategoryId;
  label: string;
  badge: string;
  description: string;
  setupTitle: string;
  setupBody: string;
  primaryAction: string;
  primaryRecipeId: string;
  recipeIds: readonly string[];
}[] = [
  {
    id: "terminal",
    label: "Terminal",
    badge: "Recommended",
    description: "Set up terminal, AI agent, and editor activity in one place.",
    setupTitle: "Install Stint",
    setupBody:
      "One command creates your key, installs Stint, writes config, and checks the connection.",
    primaryAction: "Install Stint",
    primaryRecipeId: "stint-cli-config",
    recipeIds: ["stint-cli-config"],
  },
  {
    id: "agents",
    label: "AI agents",
    badge: "Codex and Claude",
    description: "Track coding sessions from Codex or Claude Code.",
    setupTitle: "Install agent plugin",
    setupBody:
      "Choose Codex or Claude Code below, then install the Stint marketplace plugin.",
    primaryAction: "Install agent plugin",
    primaryRecipeId: "codex-config",
    recipeIds: ["codex-config", "claude-code-config", "stint-cli-config"],
  },
  {
    id: "editors",
    label: "Editors",
    badge: "VS Code, JetBrains, Vim",
    description: "Use familiar editor plugins with your Stint endpoint and key.",
    setupTitle: "Install editor plugin",
    setupBody:
      "Choose your editor below, then point the plugin at your Stint endpoint and key.",
    primaryAction: "Install editor plugin",
    primaryRecipeId: "vscode-config",
    recipeIds: ["vscode-config", "jetbrains-config", "vim-config"],
  },
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
  const [activeToolCategory, setActiveToolCategory] =
    useState<ToolCategoryId>("terminal");
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
  const latestGeneratedKeyUsed = Boolean(
    latestKeyId &&
      keys.data?.data.some((key) => key.id === latestKeyId && key.last_used_at),
  );
  const configs = useMemo(
    () => integrationConfigs(apiURL, displayKey),
    [apiURL, displayKey],
  );
  const generatedSetupCommand = stintConfiguredInstallCommand(
    apiURL,
    displayKey,
  );
  const activeCategory =
    toolCategories.find((category) => category.id === activeToolCategory) ??
    toolCategories[0];
  const connectionStatus =
    stintCLIConnected ||
    latestGeneratedKeyUsed ||
    validateMessage === "Stint is connected"
      ? "Stint is connected"
      : setupMessage || validateMessage === "Checking connection"
        ? "Waiting for first check-in"
        : validateMessage === "No check-in yet"
          ? "No check-in yet"
          : "Not connected yet";
  const connectionDetail =
    connectionStatus === "Stint is connected"
      ? `Last Stint check-in${recentStintAgent?.last_seen_at ? `: ${formatLastSeen(recentStintAgent.last_seen_at)}` : " found."}`
      : connectionStatus === "Waiting for first check-in"
        ? "Run the copied setup command, then verify the connection."
        : connectionStatus === "No check-in yet"
          ? "No Stint check-in has arrived yet. Run setup, then try again."
          : "Install Stint, then use Verify connection to confirm it is sending activity.";
  const visibleClients = clients.filter((client) =>
    activeCategory.recipeIds.includes(client.recipeId),
  );
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
        const category = categoryForRecipe(hash);
        if (category) {
          setActiveToolCategory(category.id);
        }
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
    setActiveToolCategory("terminal");
    window.history.replaceState(null, "", "#stint-cli-config");
    await copyText(
      "generated-setup",
      stintConfiguredInstallCommand(apiURL, apiKey),
    );
    setSetupMessage("Waiting for first check-in");
  };
  const openPrimarySetup = () => {
    setSelectedIntegration(activeCategory.primaryRecipeId);
    window.history.replaceState(null, "", `#${activeCategory.primaryRecipeId}`);
  };
  const runPrimaryAction = () => {
    if (activeToolCategory === "terminal") {
      void copyGeneratedSetup();
      return;
    }
    openPrimarySetup();
  };
  const validateConnection = async () => {
    setValidateMessage("Checking connection");
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
      connected ? "Stint is connected" : "No check-in yet",
    );
  };

  return (
    <div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<PlugZap size={14} />}
        caption="Stint integrations"
        title="Connect Stint"
        sub="Choose where you code. Stint will show the right setup."
        actions={
          <Link
            className="inline-flex w-fit items-center gap-2 rounded-md border border-line bg-panel px-4 py-2 text-sm text-zinc-100 hover:border-accent/50 hover:bg-white/5"
            href="/settings"
          >
            <KeyRound size={16} /> Manage keys <ArrowRight size={15} />
          </Link>
        }
      />

      <section className="mb-6">
        <h2 className="mb-2 text-lg font-semibold text-zinc-100">
          Choose where you code
        </h2>
        <div className="grid gap-3 lg:grid-cols-3">
          {toolCategories.map((category) => (
            <button
              key={category.id}
              className={`rounded border p-4 text-left transition hover:border-accent/60 hover:bg-white/5 focus:outline-none focus:ring-2 focus:ring-accent/60 ${activeToolCategory === category.id ? "border-accent/60 bg-accent/10" : "border-line bg-panel"}`}
              type="button"
              aria-pressed={activeToolCategory === category.id}
              onClick={() => {
                setActiveToolCategory(category.id);
                setSelectedIntegration(category.primaryRecipeId);
                window.history.replaceState(
                  null,
                  "",
                  `#${category.primaryRecipeId}`,
                );
              }}
            >
              <div className="mb-3 flex items-start justify-between gap-3">
                <span className="font-medium text-zinc-100">
                  {category.label}
                </span>
                <span className="rounded border border-line px-2 py-1 text-[11px] uppercase tracking-[0.14em] text-zinc-400">
                  {category.badge}
                </span>
              </div>
              <p className="text-sm leading-5 text-zinc-500">
                {category.description}
              </p>
            </button>
          ))}
        </div>
      </section>

      <section className="space-y-6">
        <div className="rounded border border-accent/35 bg-accent/10 p-4">
          <div className="mb-2 flex items-center gap-2 text-sm font-medium text-accent">
            <TerminalSquare size={16} /> {activeCategory.setupTitle}
          </div>
          <p className="mb-3 max-w-2xl text-sm leading-6 text-zinc-300">
            {activeCategory.setupBody}
          </p>
          <div className="mt-4 rounded border border-line bg-ink p-3">
            <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-500">
              Connection status
            </div>
            <div className="mt-1 text-sm font-semibold text-zinc-100">
              {connectionStatus}
            </div>
            <p className="mt-1 text-sm leading-5 text-zinc-500">
              {connectionDetail}
            </p>
          </div>
          <div className="mt-4 flex flex-col gap-2 sm:flex-row">
            <button
              className="inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-md bg-accent px-3 text-sm font-semibold text-ink hover:bg-sky-300 disabled:cursor-not-allowed disabled:opacity-60"
              type="button"
              onClick={runPrimaryAction}
              disabled={
                activeToolCategory === "terminal" &&
                createIntegrationKey.isPending
              }
            >
              {activeToolCategory === "terminal" ? (
                copied === "generated-setup" ? (
                  <Check size={15} />
                ) : (
                  <Clipboard size={15} />
                )
              ) : (
                <ArrowRight size={15} />
              )}
              {activeToolCategory === "terminal" &&
              createIntegrationKey.isPending
                ? "Creating..."
                : activeToolCategory === "terminal" &&
                    copied === "generated-setup"
                  ? "Copied"
                  : activeCategory.primaryAction}
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

        <div>
          <h2 className="text-lg font-semibold text-zinc-100">
            Choose a setup
          </h2>
          <p className="mt-1 text-sm text-zinc-500">
            Open the matching option. Details stay tucked under your selection.
          </p>

          <div className="mt-3 space-y-3">
            {visibleClients.map((client) => {
              const selected = selectedIntegration === client.recipeId;
              const config =
                configs.find((item) => item.id === client.recipeId) ??
                configs[0];
              return (
                <div key={client.name} className="rounded border border-line">
                  <ClientCard
                    {...client}
                    selected={selected}
                    onSelect={(recipeId) => {
                      setSelectedIntegration(recipeId);
                      const category = categoryForRecipe(recipeId);
                      if (category) {
                        setActiveToolCategory(category.id);
                      }
                      window.history.replaceState(null, "", `#${recipeId}`);
                    }}
                  />
                  {selected ? (
                    <SetupDisclosure
                      config={config}
                      copied={copied === config.id}
                      onCopy={() => copyText(config.id, recipeCopyText(config))}
                    />
                  ) : null}
                </div>
              );
            })}
          </div>
          <div className="mt-4 rounded border border-line bg-panel p-3 text-sm leading-6 text-zinc-500">
            Need WakaTime compatibility? Stint accepts WakaTime-style API keys
            and config files, so existing editor plugins can still send data to
            Stint. The Stint CLI is the recommended path for new installs.
          </div>
        </div>
      </section>
    </div>
  );
}

function isStintAgent(agent: UserAgent) {
  const value = `${agent.editor ?? ""} ${agent.value ?? ""}`.toLowerCase();
  return value.includes("stint");
}

function categoryForRecipe(recipeId: string) {
  return toolCategories.find((category) =>
    category.recipeIds.includes(recipeId),
  );
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
  return (
    <button
      className={`w-full rounded p-4 text-left transition hover:bg-white/5 focus:outline-none focus:ring-2 focus:ring-accent/60 ${selected ? "bg-accent/10" : "bg-panel"}`}
      type="button"
      onClick={() => onSelect(recipeId)}
      aria-label={`Show ${name} integration instructions`}
      aria-expanded={selected}
      aria-pressed={selected}
      aria-controls={selected ? "integration-instructions" : undefined}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h3 className="font-medium text-zinc-100">{name}</h3>
          <p className="mt-2 max-w-2xl text-sm leading-5 text-zinc-500">
            {description}
          </p>
        </div>
        <span className="w-fit shrink-0 rounded border border-line px-2 py-1 text-[11px] uppercase tracking-[0.14em] text-zinc-400">
          {selected ? "Open" : status}
        </span>
      </div>
    </button>
  );
}

function SetupDisclosure({
  config,
  copied,
  onCopy,
}: {
  config: IntegrationConfig;
  copied: boolean;
  onCopy: () => void;
}) {
  const [open, setOpen] = useState(false);
  const disclosureLabel =
    config.id === "stint-cli-config" ? "Show command" : "Show setup details";

  return (
    <div
      id="integration-instructions"
      className="border-t border-line bg-panel p-4"
      aria-label="integration-instructions"
    >
      <span id={config.id} className="sr-only">
        {config.name}
      </span>
      <div className="flex items-center justify-between gap-3">
        <button
          className="text-left text-sm font-medium text-zinc-100 hover:text-accent focus:outline-none focus:ring-2 focus:ring-accent/60"
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((current) => !current)}
        >
          {open ? "Hide setup details" : disclosureLabel}
        </button>
        <CopyButton
          id={config.id}
          label="Copy"
          copied={copied}
          onCopy={onCopy}
        />
      </div>
      {open ? (
        <div className="mt-4 space-y-3">
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
      ) : null}
    </div>
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
