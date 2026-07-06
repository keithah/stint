"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Save } from "lucide-react";
import { useState } from "react";
import { me, updateUser, type ProfileLayout, type PublicProfileFields, type PublicProjectVisibility, type StatsRange } from "@/lib/api";
import { rangeOptions } from "@/lib/ranges";
import { PrivacyToggle, ProfileField, type PublicProfileDraft } from "@/components/settings/shared";

const PROFILE_THEMES: Array<{ value: ProfileLayout; title: string; detail: string; preview: string }> = [
  { value: "terminal", title: "Terminal dossier", detail: "Dense, monospace, dev-native.", preview: "› keithah\n  US · open\n  ▦▦▧▩▦▩▦▩" },
  { value: "spotlight", title: "Spotlight", detail: "Gradient hero, one big number.", preview: "░░░░░░░░\n   ◯\n 72h 31m" },
  { value: "rail", title: "Split rail", detail: "Sticky identity + activity.", preview: "│ ◯ │ ▁▃▅█▆\n│ KH│ ▦▦▧▩▦\n│ US│ Go·TS" }
];

export function ProfileCard() {
  const client = useQueryClient();
  const user = useQuery({ queryKey: ["me"], queryFn: me, });
  const [profileDraft, setProfileDraft] = useState<PublicProfileDraft | null>(null);
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
    public_show_summaries: user.data?.data.public_show_summaries ?? true,
    public_profile: user.data?.data.public_profile ?? { layout: "terminal", default_range: "last_7_days" }
  };
  const pp = profile.public_profile ?? { layout: "terminal", default_range: "last_7_days" };
  const setPP = (patch: Partial<PublicProfileFields>) => setProfileDraft({ ...profile, public_profile: { ...pp, ...patch } });
  const profileFieldPublic = (key: string) => (pp.visibility?.[key] ?? "public") === "public";
  const setProfileFieldPublic = (key: string, isPublic: boolean) => {
    const visibility = { ...(pp.visibility ?? {}) };
    if (isPublic) {
      delete visibility[key];
    } else {
      visibility[key] = "private";
    }
    setPP({ visibility });
  };
  const publicHandle = (profile.public_username?.trim() || user.data?.data.github_username || "username").replace(/^@/, "");
  const publicProfilePath = `/@${publicHandle}`;
  const canSaveProfile = profile.timezone.trim().length > 0 && Number.isFinite(profile.timeout_minutes) && profile.timeout_minutes >= 0 && profile.timeout_minutes <= 120 && Number.isFinite(profile.heartbeat_retention_days) && profile.heartbeat_retention_days >= 0 && (!profile.country?.trim() || /^[A-Za-z]{2}$/.test(profile.country.trim())) && (!profile.public_username?.trim() || /^[A-Za-z0-9][A-Za-z0-9_-]{1,37}[A-Za-z0-9]$/.test(profile.public_username.trim().replace(/^@/, "")));
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
        public_show_summaries: profile.public_show_summaries,
        public_profile: profile.public_profile
      }),
    onSuccess: () => {
      setProfileDraft(null);
      client.invalidateQueries({ queryKey: ["me"] });
    }
  });

  return (
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
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <code className="min-w-0 flex-1 break-all rounded border border-line bg-panel px-3 py-2 text-xs text-zinc-400">{publicProfilePath}</code>
              <a
                href={publicProfilePath}
                target="_blank"
                rel="noreferrer"
                className="inline-flex shrink-0 items-center gap-2 rounded border border-accent/40 bg-accent/10 px-3 py-2 text-sm text-accent transition hover:bg-accent/20"
              >
                <ExternalLink size={15} /> Preview profile
              </a>
            </div>
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
          <label className="block">
            <span className="text-sm text-zinc-400">Default range</span>
            <select
              className="mt-2 w-full rounded border border-line bg-panel px-3 py-2 text-sm outline-none focus:border-accent"
              value={pp.default_range ?? "last_7_days"}
              onChange={(event) => setPP({ default_range: event.target.value as StatsRange })}
            >
              {rangeOptions.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
        </div>

        <div className="mt-5">
          <h4 className="text-sm font-medium text-zinc-300">Profile theme</h4>
          <p className="mt-1 text-xs text-zinc-500">Visitors to /@{publicHandle} see this layout. Change it anytime.</p>
          <div className="mt-3 grid gap-3 sm:grid-cols-3">
            {PROFILE_THEMES.map((theme) => {
              const active = (pp.layout ?? "terminal") === theme.value;
              return (
                <button
                  key={theme.value}
                  type="button"
                  onClick={() => setPP({ layout: theme.value })}
                  className={`rounded border p-3 text-left transition ${active ? "border-accent bg-accent/10" : "border-line bg-panel hover:bg-white/5"}`}
                >
                  <span className="block text-sm font-medium text-zinc-100">{theme.title}</span>
                  <span className="mt-1 block text-xs text-zinc-500">{theme.detail}</span>
                  <span className="mt-3 block whitespace-pre font-mono text-[10px] leading-tight text-zinc-600">{theme.preview}</span>
                </button>
              );
            })}
          </div>
        </div>

        <div className="mt-5">
          <h4 className="text-sm font-medium text-zinc-300">Personal info</h4>
          <p className="mt-1 text-xs text-zinc-500">Everything here is public by default. Toggle any field to private to hide it.</p>
          <div className="mt-3 grid gap-4 md:grid-cols-2">
            <div className="md:col-span-2">
              <ProfileField label="Bio" textarea placeholder="A sentence or two about you." value={pp.bio ?? ""} onChange={(value) => setPP({ bio: value })} isPublic={profileFieldPublic("bio")} onVisibility={(isPublic) => setProfileFieldPublic("bio", isPublic)} />
            </div>
            <ProfileField label="Location" placeholder="San Francisco, CA" value={pp.location ?? ""} onChange={(value) => setPP({ location: value })} isPublic={profileFieldPublic("location")} onVisibility={(isPublic) => setProfileFieldPublic("location", isPublic)} />
            <ProfileField label="Pronouns" placeholder="they/them" value={pp.pronouns ?? ""} onChange={(value) => setPP({ pronouns: value })} isPublic={profileFieldPublic("pronouns")} onVisibility={(isPublic) => setProfileFieldPublic("pronouns", isPublic)} />
            <ProfileField label="Company" placeholder="Acme, Inc." value={pp.company ?? ""} onChange={(value) => setPP({ company: value })} isPublic={profileFieldPublic("company")} onVisibility={(isPublic) => setProfileFieldPublic("company", isPublic)} />
            <ProfileField label="Role / title" placeholder="Staff Engineer" value={pp.role ?? ""} onChange={(value) => setPP({ role: value })} isPublic={profileFieldPublic("role")} onVisibility={(isPublic) => setProfileFieldPublic("role", isPublic)} />
            <ProfileField label="Website" placeholder="https://example.com" value={pp.website_url ?? ""} onChange={(value) => setPP({ website_url: value })} isPublic={profileFieldPublic("website")} onVisibility={(isPublic) => setProfileFieldPublic("website", isPublic)} />
            <ProfileField label="Twitter / X" placeholder="handle" prefix="@" value={pp.twitter_username ?? ""} onChange={(value) => setPP({ twitter_username: value })} isPublic={profileFieldPublic("twitter")} onVisibility={(isPublic) => setProfileFieldPublic("twitter", isPublic)} />
            <ProfileField label="LinkedIn" placeholder="https://linkedin.com/in/you" value={pp.linkedin_url ?? ""} onChange={(value) => setPP({ linkedin_url: value })} isPublic={profileFieldPublic("linkedin")} onVisibility={(isPublic) => setProfileFieldPublic("linkedin", isPublic)} />
            <ProfileField label="Mastodon" placeholder="https://mastodon.social/@you" value={pp.mastodon_url ?? ""} onChange={(value) => setPP({ mastodon_url: value })} isPublic={profileFieldPublic("mastodon")} onVisibility={(isPublic) => setProfileFieldPublic("mastodon", isPublic)} />
          </div>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <PrivacyToggle label="Available for hire" detail="Show an 'open to work' badge." checked={pp.available_for_hire ?? false} onChange={(checked) => setPP({ available_for_hire: checked })} />
            <PrivacyToggle label="Show email" detail={`Display ${user.data?.data.email || "your account email"} publicly.`} checked={pp.email_public ?? false} onChange={(checked) => setPP({ email_public: checked })} />
          </div>
        </div>

        <h4 className="mt-5 text-sm font-medium text-zinc-300">Visible sections</h4>
        <div className="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
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
  );
}
