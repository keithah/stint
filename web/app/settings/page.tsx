"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Download, KeyRound, LogOut, Plus, RotateCcw, Save, Trash2 } from "lucide-react";
import { useMemo, useState, useSyncExternalStore } from "react";
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";
import { abortCustomRulesProgress, createDataDump, createKey, createOAuthApp, createShareToken, customRulesProgress, dataDumpDownloadURL, deleteCurrentUser, deleteCustomRule, deleteOAuthApp, deleteShareToken, importWakaTimeDump, listAICosts, listCustomRules, listDataDumps, listEditors, listKeys, listOAuthApps, listShareTokens, logout, me, replaceAICosts, replaceCustomRules, revokeKey, serverMeta, updateUser, wakatimeAPIURL, type PublicProjectVisibility } from "@/lib/api";
import { boundedPercent } from "@/lib/chart-percent";
import { dataDumpExpiryText, dataDumpIsDownloadable, hasPendingDumps } from "@/lib/data-dumps";

const ruleFields = ["entity", "type", "category", "project", "branch", "language", "editor", "operating_system"];
const ruleOperations = [
  { value: "equals", label: "Equals" },
  { value: "contains", label: "Contains" },
  { value: "starts_with", label: "Starts with" },
  { value: "ends_with", label: "Ends with" },
  { value: "regex", label: "Regex" }
];

type PublicProfileDraft = {
  timezone: string;
  timeout_minutes: number;
  writes_only: boolean;
  has_public_profile: boolean;
  country?: string;
  heartbeat_retention_days: number;
  public_username?: string;
  public_display_name?: string;
  public_github_link_enabled: boolean;
  public_show_total_time: boolean;
  public_show_projects: boolean;
  public_project_visibility: PublicProjectVisibility;
  public_show_languages: boolean;
  public_show_editors: boolean;
  public_show_machines: boolean;
  public_show_operating_systems: boolean;
  public_show_categories: boolean;
  public_show_ai: boolean;
  public_show_summaries: boolean;
};

export default function SettingsPage() {
  return (
    <Providers>
      <Shell>
        <SettingsContent />
      </Shell>
    </Providers>
  );
}

function SettingsContent() {
  const client = useQueryClient();
  const [latestKey, setLatestKey] = useState("");
  const [latestOAuthSecret, setLatestOAuthSecret] = useState("");
  const [latestShareToken, setLatestShareToken] = useState("");
  const [name, setName] = useState("Workstation");
  const [keyScopes, setKeyScopes] = useState("");
  const [shareName, setShareName] = useState("Public dashboard");
  const [importFile, setImportFile] = useState<File | null>(null);
  const [settingsDumpType, setSettingsDumpType] = useState<"heartbeats" | "daily">("heartbeats");
  const [costAgent, setCostAgent] = useState("Codex");
  const [inputCost, setInputCost] = useState(3);
  const [outputCost, setOutputCost] = useState(12);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [oauthName, setOAuthName] = useState("Local OAuth client");
  const [oauthRedirect, setOAuthRedirect] = useState("http://localhost:3000/oauth/callback");
  const [oauthScopes, setOAuthScopes] = useState("read_stats read_summaries write_heartbeats");
  const [profileDraft, setProfileDraft] = useState<PublicProfileDraft | null>(null);
  const [ruleAction, setRuleAction] = useState<"change" | "delete">("change");
  const [ruleSource, setRuleSource] = useState("entity");
  const [ruleOperation, setRuleOperation] = useState("contains");
  const [ruleSourceValue, setRuleSourceValue] = useState("legacy");
  const [ruleDestination, setRuleDestination] = useState("project");
  const [ruleDestinationValue, setRuleDestinationValue] = useState("modernized");
  const [rulePriority, setRulePriority] = useState(1);
  const user = useQuery({ queryKey: ["me"], queryFn: me, retry: false });
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: listKeys, retry: false });
  const meta = useQuery({ queryKey: ["server-meta"], queryFn: serverMeta, retry: false, staleTime: 60000 });
  const editors = useQuery({ queryKey: ["editors"], queryFn: listEditors, retry: false, staleTime: 3600000 });
  const oauthApps = useQuery({ queryKey: ["oauth-apps"], queryFn: listOAuthApps, retry: false });
  const shareTokens = useQuery({ queryKey: ["share-tokens"], queryFn: listShareTokens, retry: false });
  const aiCosts = useQuery({ queryKey: ["ai-costs"], queryFn: listAICosts, retry: false });
  const customRules = useQuery({ queryKey: ["custom-rules"], queryFn: listCustomRules, retry: false });
  const settingsDumps = useQuery({
    queryKey: ["settings-data-dumps"],
    queryFn: listDataDumps,
    retry: false,
    refetchInterval: (query) => (hasPendingDumps(query.state.data) ? 2000 : false)
  });
  const ruleProgress = useQuery({ queryKey: ["custom-rules-progress"], queryFn: customRulesProgress, retry: false, refetchInterval: 2000 });
  const apiURL = useSyncExternalStore(noopSubscribe, wakatimeAPIURL, serverWakaTimeAPIURL);
  const profile = profileDraft ?? {
    timezone: user.data?.data.timezone ?? "UTC",
    timeout_minutes: user.data?.data.timeout_minutes ?? 15,
    writes_only: user.data?.data.writes_only ?? false,
    has_public_profile: user.data?.data.has_public_profile ?? false,
    country: user.data?.data.country ?? "",
    heartbeat_retention_days: user.data?.data.heartbeat_retention_days ?? 0,
    public_username: user.data?.data.public_username ?? "",
    public_display_name: user.data?.data.public_display_name ?? "",
    public_github_link_enabled: user.data?.data.public_github_link_enabled ?? false,
    public_show_total_time: user.data?.data.public_show_total_time ?? true,
    public_show_projects: user.data?.data.public_show_projects ?? true,
    public_project_visibility: user.data?.data.public_project_visibility ?? "public_repos",
    public_show_languages: user.data?.data.public_show_languages ?? true,
    public_show_editors: user.data?.data.public_show_editors ?? false,
    public_show_machines: user.data?.data.public_show_machines ?? false,
    public_show_operating_systems: user.data?.data.public_show_operating_systems ?? false,
    public_show_categories: user.data?.data.public_show_categories ?? false,
    public_show_ai: user.data?.data.public_show_ai ?? false,
    public_show_summaries: user.data?.data.public_show_summaries ?? true
  };
  const publicOrigin = typeof window === "undefined" ? "" : window.location.origin;
  const publicHandle = (profile.public_username?.trim() || user.data?.data.github_username || "username").replace(/^@/, "");
  const publicProfileURL = `${publicOrigin}/@${publicHandle}`;
  const oauthRedirectURIs = oauthRedirect.split("\n").map((value) => value.trim()).filter(Boolean);
  const canCreateAPIKey = name.trim().length > 0;
  const canCreateOAuthApp = oauthName.trim().length > 0 && oauthRedirectURIs.length > 0 && oauthRedirectURIs.every(isHTTPURL);
  const canCreateShareToken = shareName.trim().length > 0;
  const canSaveProfile = profile.timezone.trim().length > 0 && Number.isFinite(profile.timeout_minutes) && profile.timeout_minutes >= 0 && profile.timeout_minutes <= 120 && Number.isFinite(profile.heartbeat_retention_days) && profile.heartbeat_retention_days >= 0 && (!profile.country?.trim() || /^[A-Za-z]{2}$/.test(profile.country.trim())) && (!profile.public_username?.trim() || /^[A-Za-z0-9][A-Za-z0-9_-]{1,37}[A-Za-z0-9]$/.test(profile.public_username.trim().replace(/^@/, "")));
  const canSaveAICosts = costAgent.trim().length > 0 && Number.isFinite(inputCost) && Number.isFinite(outputCost) && inputCost >= 0 && outputCost >= 0;
  const canSaveCustomRule = ruleSourceValue.trim().length > 0 && Number.isFinite(rulePriority) && rulePriority >= 1 && (ruleAction === "delete" || ruleDestinationValue.trim().length > 0);
  const create = useMutation({
    mutationFn: (keyName: string) => createKey(keyName, keyScopes.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean)),
    onSuccess: (result) => {
      setLatestKey(result.data.api_key);
      client.invalidateQueries({ queryKey: ["api-keys"] });
    }
  });
  const revoke = useMutation({
    mutationFn: revokeKey,
    onSuccess: () => client.invalidateQueries({ queryKey: ["api-keys"] })
  });
  const createApp = useMutation({
    mutationFn: () =>
      createOAuthApp({
        name: oauthName.trim(),
        redirect_uris: oauthRedirectURIs,
        scopes: oauthScopes.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean)
      }),
    onSuccess: (result) => {
      setLatestOAuthSecret(result.data.client_secret ?? "");
      client.invalidateQueries({ queryKey: ["oauth-apps"] });
    }
  });
  const deleteApp = useMutation({
    mutationFn: deleteOAuthApp,
    onSuccess: () => client.invalidateQueries({ queryKey: ["oauth-apps"] })
  });
  const createShare = useMutation({
    mutationFn: () => createShareToken(shareName.trim()),
    onSuccess: (result) => {
      setLatestShareToken(result.data.token ?? "");
      client.invalidateQueries({ queryKey: ["share-tokens"] });
    }
  });
  const removeShare = useMutation({
    mutationFn: deleteShareToken,
    onSuccess: () => client.invalidateQueries({ queryKey: ["share-tokens"] })
  });
  const importDump = useMutation({
    mutationFn: () => {
      if (!importFile) {
        throw new Error("Choose an activity JSON dump first");
      }
      return importWakaTimeDump(importFile);
    }
  });
  const createSettingsDump = useMutation({
    mutationFn: () => createDataDump(settingsDumpType),
    onSuccess: () => client.invalidateQueries({ queryKey: ["settings-data-dumps"] })
  });
  const saveCosts = useMutation({
    mutationFn: () => replaceAICosts([{ agent: costAgent.trim(), input_cost_per_million_cents: inputCost, output_cost_per_million_cents: outputCost }]),
    onSuccess: () => client.invalidateQueries({ queryKey: ["ai-costs"] })
  });
  const deleteAccount = useMutation({
    mutationFn: () => deleteCurrentUser(deleteConfirmation),
    onSuccess: () => {
      window.location.href = "/login";
    }
  });
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      window.location.href = "/login";
    }
  });
  const saveProfile = useMutation({
    mutationFn: () =>
      updateUser({
        ...profile,
        timezone: profile.timezone.trim(),
        country: profile.country?.trim().toUpperCase(),
        timeout_minutes: Math.min(120, Math.max(0, profile.timeout_minutes)),
        heartbeat_retention_days: Math.max(0, profile.heartbeat_retention_days),
        public_username: profile.public_username?.trim().replace(/^@/, ""),
        public_display_name: profile.public_display_name?.trim(),
        public_github_link_enabled: profile.public_github_link_enabled,
        public_show_total_time: profile.public_show_total_time,
        public_show_projects: profile.public_show_projects,
        public_project_visibility: profile.public_project_visibility,
        public_show_languages: profile.public_show_languages,
        public_show_editors: profile.public_show_editors,
        public_show_machines: profile.public_show_machines,
        public_show_operating_systems: profile.public_show_operating_systems,
        public_show_categories: profile.public_show_categories,
        public_show_ai: profile.public_show_ai,
        public_show_summaries: profile.public_show_summaries
      }),
    onSuccess: () => {
      setProfileDraft(null);
      client.invalidateQueries({ queryKey: ["me"] });
    }
  });
  const saveRule = useMutation({
    mutationFn: () =>
      replaceCustomRules([
        ...(customRules.data?.data ?? []),
        {
          action: ruleAction,
          source: ruleSource,
          operation: ruleOperation,
          source_value: ruleSourceValue.trim(),
          priority: rulePriority,
          destinations: ruleAction === "change" ? [{ destination: ruleDestination, destination_value: ruleDestinationValue.trim() }] : []
        }
      ]),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["custom-rules"] });
      client.invalidateQueries({ queryKey: ["custom-rules-progress"] });
    }
  });
  const removeCustomRule = useMutation({
    mutationFn: deleteCustomRule,
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["custom-rules"] });
      client.invalidateQueries({ queryKey: ["custom-rules-progress"] });
    }
  });
  const abortRuleProgress = useMutation({
    mutationFn: abortCustomRulesProgress,
    onSuccess: () => client.invalidateQueries({ queryKey: ["custom-rules-progress"] })
  });
  const configBlock = useMemo(
    () => `[settings]\napi_url = ${apiURL}\napi_key = ${latestKey || "waka_00000000-0000-4000-8000-000000000000"}\nhide_file_names = false\ntimeout = ${profile.timeout_minutes}`,
    [apiURL, latestKey, profile.timeout_minutes]
  );
  const fanoutConfigBlock = useMemo(
    () => `[api_urls]\n.* = ${apiURL}|${latestKey || "waka_00000000-0000-4000-8000-000000000000"}`,
    [apiURL, latestKey]
  );

  return (
    <div className="mx-auto max-w-5xl px-5 py-6 lg:px-8">
      <header className="mb-8 border-b border-line pb-6">
        <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
          <KeyRound size={14} /> Stint config
        </div>
        <h1 className="text-4xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-2 text-sm text-zinc-400">Manage profile privacy, API keys, editor setup, imports, sharing, and AI cost settings.</p>
      </header>

      <section className="mb-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
          <div className="flex min-w-0 items-center gap-4">
            <div
              className="flex h-14 w-14 shrink-0 items-center justify-center rounded border border-line bg-ink bg-cover bg-center text-lg font-semibold text-zinc-300"
              style={user.data?.data.avatar_url ? { backgroundImage: `url(${user.data.data.avatar_url})` } : undefined}
              aria-hidden="true"
            >
              {user.data?.data.avatar_url ? "" : (user.data?.data.github_username ?? "?").slice(0, 1).toUpperCase()}
            </div>
            <div className="min-w-0">
              <h2 className="font-medium">GitHub account</h2>
              <p className="mt-1 truncate text-sm text-zinc-400">{user.data?.data.full_name || user.data?.data.github_username || "Loading account"}</p>
              <p className="mt-1 truncate text-xs text-zinc-500">
                @{user.data?.data.github_username ?? "unknown"}
                {user.data?.data.email ? ` · ${user.data.data.email}` : ""}
              </p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <div className="rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-500">GitHub SSO</div>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5 disabled:opacity-60"
              onClick={() => signOut.mutate()}
              disabled={signOut.isPending}
            >
              <LogOut size={16} /> Sign out
            </button>
          </div>
        </div>
      </section>

      <section className="mb-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <h2 className="font-medium">Profile preferences</h2>
            <p className="mt-1 text-sm text-zinc-400">These settings affect duration merging and computed dashboard totals.</p>
          </div>
          <button
            className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60"
            onClick={() => saveProfile.mutate()}
            disabled={saveProfile.isPending || !canSaveProfile}
          >
            <Save size={16} /> Save preferences
          </button>
        </div>
        <div className="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-6">
          <label className="block">
            <span className="text-sm text-zinc-400">Timezone</span>
            <input
              className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              value={profile.timezone}
              onChange={(event) => setProfileDraft({ ...profile, timezone: event.target.value })}
            />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Timeout minutes</span>
            <input
              className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              type="number"
              min={0}
              max={120}
              value={profile.timeout_minutes}
              onChange={(event) => setProfileDraft({ ...profile, timeout_minutes: Math.min(120, Math.max(0, Number(event.target.value) || 0)) })}
            />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Country</span>
            <input
              className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm uppercase outline-none focus:border-accent"
              maxLength={2}
              placeholder="US"
              value={profile.country ?? ""}
              onChange={(event) => setProfileDraft({ ...profile, country: event.target.value.toUpperCase() })}
            />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Retention days</span>
            <input
              className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              type="number"
              min={0}
              value={profile.heartbeat_retention_days}
              onChange={(event) => setProfileDraft({ ...profile, heartbeat_retention_days: Math.max(0, Number(event.target.value) || 0) })}
            />
          </label>
          <label className="flex items-center justify-between gap-4 rounded border border-line bg-ink px-3 py-2">
            <span>
              <span className="block text-sm text-zinc-300">Writes only</span>
              <span className="text-xs text-zinc-500">Use only write heartbeats in computed stats and summaries.</span>
            </span>
            <input className="h-5 w-5 accent-accent" type="checkbox" checked={profile.writes_only} onChange={(event) => setProfileDraft({ ...profile, writes_only: event.target.checked })} />
          </label>
        </div>
        <div className="mt-5 rounded border border-line bg-ink p-4">
          <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
            <div>
              <h3 className="font-medium text-zinc-100">Public profile</h3>
              <p className="mt-1 text-sm text-zinc-500">Publish a clean profile URL and explicitly choose which activity sections are visible.</p>
              <code className="mt-3 block break-all rounded border border-line bg-panel px-3 py-2 text-xs text-zinc-400">{publicProfileURL}</code>
            </div>
            <label className="flex items-center justify-between gap-4 rounded border border-line bg-panel px-3 py-2">
              <span>
                <span className="block text-sm text-zinc-300">Profile enabled</span>
                <span className="text-xs text-zinc-500">Allow public reads at /@{publicHandle}.</span>
              </span>
              <input className="h-5 w-5 accent-accent" type="checkbox" checked={profile.has_public_profile} onChange={(event) => setProfileDraft({ ...profile, has_public_profile: event.target.checked })} />
            </label>
          </div>

          <div className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <label className="block">
              <span className="text-sm text-zinc-400">Public username</span>
              <div className="mt-2 flex rounded border border-line bg-panel focus-within:border-accent">
                <span className="border-r border-line px-3 py-2 text-sm text-zinc-500">@</span>
                <input
                  className="min-w-0 flex-1 bg-transparent px-3 py-2 text-sm outline-none"
                  placeholder="keith"
                  value={profile.public_username ?? ""}
                  onChange={(event) => setProfileDraft({ ...profile, public_username: event.target.value })}
                />
              </div>
            </label>
            <label className="block">
              <span className="text-sm text-zinc-400">Display name</span>
              <input
                className="mt-2 w-full rounded border border-line bg-panel px-3 py-2 text-sm outline-none focus:border-accent"
                placeholder={user.data?.data.full_name || user.data?.data.github_username || "Keith"}
                value={profile.public_display_name ?? ""}
                onChange={(event) => setProfileDraft({ ...profile, public_display_name: event.target.value })}
              />
            </label>
            <label className="block">
              <span className="text-sm text-zinc-400">Project visibility</span>
              <select
                className="mt-2 w-full rounded border border-line bg-panel px-3 py-2 text-sm outline-none focus:border-accent"
                value={profile.public_project_visibility}
                onChange={(event) => setProfileDraft({ ...profile, public_project_visibility: event.target.value as PublicProjectVisibility })}
              >
                <option value="none">No project names</option>
                <option value="public_repos">Public GitHub repos only</option>
                <option value="all">All project names</option>
              </select>
            </label>
          </div>

          <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <PrivacyToggle label="GitHub link" detail="Show your GitHub username, avatar, and profile link." checked={profile.public_github_link_enabled} onChange={(checked) => setProfileDraft({ ...profile, public_github_link_enabled: checked })} />
            <PrivacyToggle label="Total time" detail="Show totals, best day, averages, and activity bars." checked={profile.public_show_total_time} onChange={(checked) => setProfileDraft({ ...profile, public_show_total_time: checked })} />
            <PrivacyToggle label="Projects" detail="Show project totals according to project visibility." checked={profile.public_show_projects} onChange={(checked) => setProfileDraft({ ...profile, public_show_projects: checked })} />
            <PrivacyToggle label="Languages" detail="Show language breakdowns and language timelines." checked={profile.public_show_languages} onChange={(checked) => setProfileDraft({ ...profile, public_show_languages: checked })} />
            <PrivacyToggle label="AI metrics" detail="Show AI lines, sessions, tokens, costs, and agents." checked={profile.public_show_ai} onChange={(checked) => setProfileDraft({ ...profile, public_show_ai: checked })} />
            <PrivacyToggle label="Summaries" detail="Enable public date-range summaries." checked={profile.public_show_summaries} onChange={(checked) => setProfileDraft({ ...profile, public_show_summaries: checked })} />
            <PrivacyToggle label="Editors" detail="Show editor/plugin breakdowns." checked={profile.public_show_editors} onChange={(checked) => setProfileDraft({ ...profile, public_show_editors: checked })} />
            <PrivacyToggle label="Machines" detail="Show machine-name breakdowns." checked={profile.public_show_machines} onChange={(checked) => setProfileDraft({ ...profile, public_show_machines: checked })} />
            <PrivacyToggle label="Operating systems" detail="Show OS breakdowns." checked={profile.public_show_operating_systems} onChange={(checked) => setProfileDraft({ ...profile, public_show_operating_systems: checked })} />
            <PrivacyToggle label="Categories" detail="Show coding/debugging/building categories." checked={profile.public_show_categories} onChange={(checked) => setProfileDraft({ ...profile, public_show_categories: checked })} />
          </div>
        </div>
        {saveProfile.error ? <p className="mt-3 text-sm text-red-300">{saveProfile.error.message}</p> : null}
      </section>

      <section className="grid gap-5 lg:grid-cols-[1fr_1fr]">
        <div className="rounded border border-line bg-panel p-5">
          <h2 className="font-medium">Create API key</h2>
          <div className="mt-4 grid gap-2">
            <input
              className="min-w-0 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            <div className="flex gap-2">
              <input
                className="min-w-0 flex-1 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
                placeholder="Scopes, blank for full access"
                value={keyScopes}
                onChange={(event) => setKeyScopes(event.target.value)}
              />
              <button
                className="inline-flex items-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60"
                onClick={() => create.mutate(name.trim())}
                disabled={create.isPending || !canCreateAPIKey}
              >
                <Plus size={16} /> Create
              </button>
            </div>
          </div>
          {latestKey ? (
            <div className="mt-4 rounded border border-accent/40 bg-accent/10 p-3">
              <div className="text-xs uppercase tracking-[0.16em] text-accent">Shown once</div>
              <code className="mt-2 block break-all text-sm text-zinc-100">{latestKey}</code>
            </div>
          ) : null}
        </div>

        <div className="rounded border border-line bg-panel p-5">
          <div className="flex items-center justify-between gap-3">
            <h2 className="font-medium">Editor config file</h2>
            <button
              className="inline-flex items-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5"
              onClick={() => navigator.clipboard.writeText(configBlock)}
            >
              <Copy size={15} /> Copy
            </button>
          </div>
          <pre className="mt-4 overflow-x-auto rounded border border-line bg-ink p-4 text-sm leading-6 text-zinc-200">{configBlock}</pre>
        </div>

        <div className="rounded border border-line bg-panel p-5">
          <div className="flex items-center justify-between gap-3">
            <h2 className="font-medium">api_urls fanout</h2>
            <button
              className="inline-flex items-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5"
              onClick={() => navigator.clipboard.writeText(fanoutConfigBlock)}
            >
              <Copy size={15} /> Copy
            </button>
          </div>
          <p className="mt-2 text-sm text-zinc-400">Use this form when sending the same Codex or editor activity to multiple services.</p>
          <pre className="mt-4 overflow-x-auto rounded border border-line bg-ink p-4 text-sm leading-6 text-zinc-200">{fanoutConfigBlock}</pre>
        </div>
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div>
            <h2 className="font-medium">Server diagnostics</h2>
            <p className="mt-1 text-sm text-zinc-400">Confirm the public API origin and runtime details reported to connected clients.</p>
          </div>
          <code className="rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-500">GET /api/v1/meta</code>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          <Diagnostic label="API URL" value={meta.data?.data.api_url ?? apiURL} />
          <Diagnostic label="Base URL" value={meta.data?.data.base_url ?? "Loading"} />
          <Diagnostic label="Hostname" value={meta.data?.data.hostname ?? "Loading"} />
          <Diagnostic label="Client IP" value={meta.data?.data.ip ?? "Loading"} />
          <Diagnostic label="Version" value={meta.data?.data.version ?? "Loading"} />
        </div>
        {meta.error ? <p className="mt-3 text-sm text-red-300">{meta.error.message}</p> : null}
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div>
            <h2 className="font-medium">Supported editors</h2>
            <p className="mt-1 text-sm text-zinc-400">Known editor clients exposed by the local metadata endpoint.</p>
          </div>
          <code className="rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-500">GET /api/v1/editors</code>
        </div>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {(editors.data?.data ?? []).slice(0, 8).map((editor) => (
            <div key={editor.key} className="rounded border border-line bg-ink p-3">
              <div className="font-medium text-zinc-100">{editor.name}</div>
              <div className="mt-1 text-xs text-zinc-500">{editor.key}</div>
              {editor.version ? <div className="mt-2 rounded border border-line px-2 py-1 text-xs text-zinc-400">{editor.version}</div> : null}
            </div>
          ))}
          {editors.data?.data.length === 0 ? <div className="rounded border border-line bg-ink p-3 text-sm text-zinc-500">No editor metadata returned yet.</div> : null}
        </div>
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <h2 className="font-medium">API keys</h2>
        <div className="mt-4 divide-y divide-line rounded border border-line">
          {(keys.data?.data ?? []).map((key) => (
            <div key={key.id} className="flex flex-col justify-between gap-3 p-4 sm:flex-row sm:items-center">
              <div>
                <div className="font-medium text-zinc-100">{key.name}</div>
                <div className="mt-1 text-sm text-zinc-500">Fingerprint {key.fingerprint}</div>
                <div className="mt-2 flex flex-wrap gap-1">
                  {key.scopes.map((scope) => (
                    <span key={scope} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">{scope}</span>
                  ))}
                </div>
              </div>
              <button
                className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40"
                onClick={() => revoke.mutate(key.id)}
              >
                <Trash2 size={15} /> Revoke
              </button>
            </div>
          ))}
          {keys.data?.data?.length === 0 ? <div className="p-4 text-sm text-zinc-500">No API keys yet.</div> : null}
        </div>
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div>
            <h2 className="font-medium">OAuth applications</h2>
            <p className="mt-1 text-sm text-zinc-400">Register external clients for authorization-code and refresh-token flows.</p>
          </div>
          <button
            className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60"
            onClick={() => createApp.mutate()}
            disabled={createApp.isPending || !canCreateOAuthApp}
          >
            <Plus size={16} /> Create app
          </button>
        </div>
        <div className="mt-5 grid gap-4 lg:grid-cols-[0.85fr_1.15fr]">
          <div className="space-y-3">
            <label className="block">
              <span className="text-sm text-zinc-400">App name</span>
              <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={oauthName} onChange={(event) => setOAuthName(event.target.value)} />
            </label>
            <label className="block">
              <span className="text-sm text-zinc-400">Redirect URIs</span>
              <textarea className="mt-2 min-h-20 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={oauthRedirect} onChange={(event) => setOAuthRedirect(event.target.value)} />
            </label>
            <label className="block">
              <span className="text-sm text-zinc-400">Scopes</span>
              <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={oauthScopes} onChange={(event) => setOAuthScopes(event.target.value)} />
            </label>
            {latestOAuthSecret ? (
              <div className="rounded border border-accent/40 bg-accent/10 p-3">
                <div className="text-xs uppercase tracking-[0.16em] text-accent">Client secret shown once</div>
                <code className="mt-2 block break-all text-sm text-zinc-100">{latestOAuthSecret}</code>
              </div>
            ) : null}
            {createApp.error ? <p className="text-sm text-red-300">{createApp.error.message}</p> : null}
          </div>
          <div className="divide-y divide-line rounded border border-line">
            {(oauthApps.data?.data ?? []).map((app) => (
              <div key={app.id} className="p-4">
                <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
                  <div className="min-w-0">
                    <div className="font-medium text-zinc-100">{app.name}</div>
                    <code className="mt-1 block break-all text-xs text-zinc-500">{app.client_id}</code>
                  </div>
                  <button className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40" onClick={() => deleteApp.mutate(app.id)}>
                    <Trash2 size={15} /> Delete
                  </button>
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {app.scopes.map((scope) => (
                    <span key={scope} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">{scope}</span>
                  ))}
                </div>
                <div className="mt-3 space-y-1">
                  {app.redirect_uris.map((uri) => (
                    <div key={uri} className="truncate text-xs text-zinc-500">{uri}</div>
                  ))}
                </div>
              </div>
            ))}
            {oauthApps.data?.data.length === 0 ? <div className="p-4 text-sm text-zinc-500">No OAuth apps yet.</div> : null}
          </div>
        </div>
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <h2 className="font-medium">Data export</h2>
            <p className="mt-1 text-sm text-zinc-400">Generate downloadable heartbeat archives or daily summary exports for backup and portability.</p>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <select className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={settingsDumpType} onChange={(event) => setSettingsDumpType(event.target.value as "heartbeats" | "daily")}>
              <option value="heartbeats">Heartbeats</option>
              <option value="daily">Daily summaries</option>
            </select>
            <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => createSettingsDump.mutate()} disabled={createSettingsDump.isPending}>
              <Plus size={16} /> Generate export
            </button>
          </div>
        </div>
        <div className="mt-4 divide-y divide-line rounded border border-line">
          {(settingsDumps.data?.data ?? []).slice(0, 8).map((dump) => {
            const isReady = dataDumpIsDownloadable(dump);
            const expiryText = dataDumpExpiryText(dump);
            return (
              <a
                key={dump.id}
                className={`flex flex-col justify-between gap-2 px-3 py-3 text-sm sm:flex-row sm:items-center ${isReady ? "hover:bg-white/5" : "cursor-not-allowed opacity-60"}`}
                href={isReady ? dataDumpDownloadURL(dump.download_url) : "#"}
                aria-disabled={!isReady}
                onClick={(event) => {
                  if (!isReady) {
                    event.preventDefault();
                  }
                }}
              >
                <span>
                  <span className="font-medium text-zinc-100">{dump.type}</span>
                  <span className="ml-2 text-zinc-500">{dump.status}</span>
                  {expiryText ? <span className="ml-2 text-zinc-600">{expiryText}</span> : null}
                  {!isReady && !expiryText ? <span className="ml-2 text-zinc-600">{dump.percent_complete}%</span> : null}
                </span>
                {isReady ? <span className="inline-flex items-center gap-2 text-zinc-300"><Download size={15} /> Download</span> : null}
              </a>
            );
          })}
          {settingsDumps.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No exports generated yet.</div> : null}
        </div>
        {createSettingsDump.error ? <p className="mt-3 text-sm text-red-300">{createSettingsDump.error.message}</p> : null}
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <h2 className="font-medium">Share tokens</h2>
            <p className="mt-1 text-sm text-zinc-400">Create read-only public stats links and JSONP-compatible embed endpoints.</p>
          </div>
          <div className="flex w-full flex-col gap-2 sm:w-auto sm:min-w-96 sm:flex-row">
            <input className="min-w-0 flex-1 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={shareName} onChange={(event) => setShareName(event.target.value)} />
            <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => createShare.mutate()} disabled={createShare.isPending || !canCreateShareToken}>
              <Plus size={16} /> Create
            </button>
          </div>
        </div>
        {latestShareToken && user.data?.data ? (
          <div className="mt-4 rounded border border-accent/40 bg-accent/10 p-3">
            <div className="text-xs uppercase tracking-[0.16em] text-accent">Share token shown once</div>
            <code className="mt-2 block break-all text-sm text-zinc-100">{latestShareToken}</code>
            <code className="mt-2 block break-all text-xs text-zinc-400">{`${publicOrigin}/share/${latestShareToken}`}</code>
            <div className="mt-3 text-xs uppercase tracking-[0.16em] text-accent">JSONP stats endpoint</div>
            <code className="mt-2 block break-all text-xs text-zinc-400">{shareStatsJSONPURL(apiURL, latestShareToken)}</code>
          </div>
        ) : null}
        <div className="mt-4 divide-y divide-line rounded border border-line">
          {(shareTokens.data?.data ?? []).map((token) => (
            <div key={token.id} className="flex flex-col justify-between gap-3 p-4 sm:flex-row sm:items-center">
              <div>
                <div className="font-medium text-zinc-100">{token.name}</div>
                <div className="mt-1 text-sm text-zinc-500">Fingerprint {token.fingerprint}</div>
              </div>
              <button className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40" onClick={() => removeShare.mutate(token.id)}>
                <Trash2 size={15} /> Delete
              </button>
            </div>
          ))}
          {shareTokens.data?.data.length === 0 ? <div className="p-4 text-sm text-zinc-500">No share tokens yet.</div> : null}
        </div>
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <h2 className="font-medium">Import activity dump</h2>
            <p className="mt-1 text-sm text-zinc-400">Upload a raw activity JSON or .json.gz dump; duplicates are skipped during import.</p>
          </div>
          <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => importDump.mutate()} disabled={!importFile || importDump.isPending}>
            <Save size={16} /> Import
          </button>
        </div>
        <input
          className="mt-4 block w-full rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-300 file:mr-4 file:rounded file:border-0 file:bg-accent file:px-3 file:py-1 file:text-sm file:font-medium file:text-ink"
          type="file"
          accept="application/json,application/gzip,.json,.json.gz,.gz"
          onChange={(event) => setImportFile(event.target.files?.[0] ?? null)}
        />
        {importDump.data ? (
          <div className="mt-4 grid gap-3 text-sm sm:grid-cols-5">
            <div className="rounded border border-line bg-ink p-3"><span className="block text-zinc-500">Status</span>{importDump.data.data.status}</div>
            <div className="rounded border border-line bg-ink p-3"><span className="block text-zinc-500">Inserted</span>{importDump.data.data.inserted}</div>
            <div className="rounded border border-line bg-ink p-3"><span className="block text-zinc-500">Duplicates</span>{importDump.data.data.duplicates}</div>
            <div className="rounded border border-line bg-ink p-3"><span className="block text-zinc-500">Invalid</span>{importDump.data.data.invalid}</div>
            <div className="rounded border border-line bg-ink p-3"><span className="block text-zinc-500">Total</span>{importDump.data.data.total}</div>
          </div>
        ) : null}
        {importDump.error ? <p className="mt-3 text-sm text-red-300">{importDump.error.message}</p> : null}
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <h2 className="font-medium">AI model cost rates</h2>
            <p className="mt-1 text-sm text-zinc-400">Estimate spend from stored AI input/output token counts by model or agent.</p>
          </div>
          <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => saveCosts.mutate()} disabled={saveCosts.isPending || !canSaveAICosts}>
            <Save size={16} /> Save rates
          </button>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-3">
          <label className="block">
            <span className="text-sm text-zinc-400">Model or agent</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={costAgent} onChange={(event) => setCostAgent(event.target.value)} />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Input cents / 1M</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} value={inputCost} onChange={(event) => setInputCost(Math.max(0, Number(event.target.value)))} />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Output cents / 1M</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} value={outputCost} onChange={(event) => setOutputCost(Math.max(0, Number(event.target.value)))} />
          </label>
        </div>
        <div className="mt-4 divide-y divide-line rounded border border-line">
          {(aiCosts.data?.data ?? []).map((setting) => (
            <div key={setting.agent} className="grid gap-2 px-3 py-3 text-sm sm:grid-cols-3">
              <span className="font-medium text-zinc-100">{setting.agent}</span>
              <span className="text-zinc-500">Input {setting.input_cost_per_million_cents}c / 1M</span>
              <span className="text-zinc-500">Output {setting.output_cost_per_million_cents}c / 1M</span>
            </div>
          ))}
          {aiCosts.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">Default rates are active.</div> : null}
        </div>
        {saveCosts.error ? <p className="mt-3 text-sm text-red-300">{saveCosts.error.message}</p> : null}
      </section>

      <section className="mt-5 rounded border border-line bg-panel p-5">
        <h2 className="font-medium">Custom rules</h2>
        <p className="mt-1 text-sm text-zinc-400">Apply personal rewrite or delete rules before heartbeats are stored, then reprocess existing rows.</p>
        <div className="mt-4 grid gap-3 lg:grid-cols-[120px_150px_150px_1fr]">
          <label className="block">
            <span className="text-sm text-zinc-400">Action</span>
            <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleAction} onChange={(event) => setRuleAction(event.target.value as "change" | "delete")}>
              <option value="change">Change</option>
              <option value="delete">Delete</option>
            </select>
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Source</span>
            <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleSource} onChange={(event) => setRuleSource(event.target.value)}>
              {ruleFields.map((field) => (
                <option key={field} value={field}>{field}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Operation</span>
            <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleOperation} onChange={(event) => setRuleOperation(event.target.value)}>
              {ruleOperations.map((operation) => (
                <option key={operation.value} value={operation.value}>{operation.label}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Match value</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleSourceValue} onChange={(event) => setRuleSourceValue(event.target.value)} />
          </label>
        </div>
        <div className="mt-3 grid gap-3 lg:grid-cols-[150px_1fr_120px_auto]">
          <label className="block">
            <span className="text-sm text-zinc-400">Destination</span>
            <select
              className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
              value={ruleDestination}
              onChange={(event) => setRuleDestination(event.target.value)}
              disabled={ruleAction === "delete"}
            >
              {ruleFields.map((field) => (
                <option key={field} value={field}>{field}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Destination value</span>
            <input
              className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
              value={ruleDestinationValue}
              onChange={(event) => setRuleDestinationValue(event.target.value)}
              disabled={ruleAction === "delete"}
            />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Priority</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={1} value={rulePriority} onChange={(event) => setRulePriority(Math.max(1, Number(event.target.value) || 1))} />
          </label>
          <button className="mt-7 inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => saveRule.mutate()} disabled={saveRule.isPending || !canSaveCustomRule}>
            <Save size={16} /> Add rule
          </button>
        </div>
        {saveRule.error ? <p className="mt-3 text-sm text-red-300">{saveRule.error.message}</p> : null}
        <div className="mt-4 divide-y divide-line rounded border border-line">
          {(customRules.data?.data ?? []).map((rule) => (
            <div key={rule.id ?? `${rule.source}-${rule.source_value}`} className="flex flex-col justify-between gap-3 px-3 py-3 text-sm sm:flex-row sm:items-center">
              <div>
                <span className="font-medium text-zinc-100">{rule.action}</span>
                <span className="ml-2 text-zinc-500">{rule.source} {rule.operation} {rule.source_value}</span>
                {rule.destinations?.length ? (
                  <div className="mt-2 flex flex-wrap gap-2">
                    {rule.destinations.map((destination) => (
                      <span key={`${destination.destination}-${destination.destination_value}`} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">
                        {destination.destination}: {destination.destination_value}
                      </span>
                    ))}
                  </div>
                ) : null}
              </div>
              {rule.id ? (
                <button className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40" onClick={() => removeCustomRule.mutate(rule.id!)} disabled={removeCustomRule.isPending}>
                  <Trash2 size={15} /> Delete
                </button>
              ) : null}
            </div>
          ))}
          {customRules.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No rules saved yet.</div> : null}
        </div>
        <div className="mt-4 rounded border border-line bg-ink p-4">
          <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
            <div>
              <div className="text-sm font-medium text-zinc-100">Retroactive apply</div>
              <div className="mt-1 text-sm text-zinc-500">
                {ruleProgress.data?.data.status ?? "NotStarted"} · {ruleProgress.data?.data.percent_complete ?? 0}% · {ruleProgress.data?.data.changed ?? 0} changed · {ruleProgress.data?.data.deleted ?? 0} deleted
              </div>
            </div>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5 disabled:opacity-60"
              onClick={() => abortRuleProgress.mutate()}
              disabled={abortRuleProgress.isPending}
            >
              <RotateCcw size={15} /> Abort job
            </button>
          </div>
          <div className="mt-3 h-2 overflow-hidden rounded bg-white/5">
            <div className="h-full rounded bg-accent" style={{ width: `${boundedPercent(ruleProgress.data?.data.percent_complete ?? 0)}%` }} />
          </div>
          {ruleProgress.data?.data.error ? <p className="mt-3 text-sm text-red-300">{ruleProgress.data.data.error}</p> : null}
        </div>
      </section>

      <section className="mt-5 rounded border border-red-900/70 bg-red-950/10 p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <h2 className="font-medium text-red-200">Danger zone</h2>
            <p className="mt-1 text-sm text-red-200/70">Delete your account, sessions, API keys, heartbeats, settings, shares, and generated data.</p>
          </div>
          <button
            className="inline-flex items-center justify-center gap-2 rounded bg-red-500 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
            onClick={() => deleteAccount.mutate()}
            disabled={deleteConfirmation !== "DELETE" || deleteAccount.isPending}
          >
            <Trash2 size={16} /> Delete account
          </button>
        </div>
        <input
          className="mt-4 w-full rounded border border-red-900/70 bg-ink px-3 py-2 text-sm outline-none focus:border-red-400"
          placeholder="DELETE"
          value={deleteConfirmation}
          onChange={(event) => setDeleteConfirmation(event.target.value)}
        />
        {deleteAccount.error ? <p className="mt-3 text-sm text-red-300">{deleteAccount.error.message}</p> : null}
      </section>
    </div>
  );
}

function Diagnostic({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded border border-line bg-ink p-3">
      <div className="text-xs uppercase tracking-[0.16em] text-zinc-500">{label}</div>
      <div className="mt-2 truncate text-sm text-zinc-200" title={value}>{value}</div>
    </div>
  );
}

function PrivacyToggle({ label, detail, checked, onChange }: { label: string; detail: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex min-h-24 items-start justify-between gap-3 rounded border border-line bg-panel px-3 py-3">
      <span>
        <span className="block text-sm font-medium text-zinc-200">{label}</span>
        <span className="mt-1 block text-xs leading-5 text-zinc-500">{detail}</span>
      </span>
      <input className="mt-1 h-5 w-5 shrink-0 accent-accent" type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
    </label>
  );
}

function noopSubscribe() {
  return () => {};
}

function serverWakaTimeAPIURL() {
  return "/api/v1";
}

function shareStatsJSONPURL(apiURL: string, token: string) {
  const query = new URLSearchParams({ range: "last_7_days", callback: "StintEmbed.render" });
  return `${apiURL}/share/${encodeURIComponent(token)}/stats?${query.toString()}`;
}

function isHTTPURL(value: string) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}
