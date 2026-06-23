"use client";

import { Briefcase, Github, Globe, Linkedin, Mail, MapPin, Twitter } from "lucide-react";
import type { ReactNode } from "react";
import { AIPanel } from "@/components/ai-panel";
import { SliceDonut } from "@/components/dashboard-charts";
import { activityHeatmapClass, activityHeatmapTitle } from "@/lib/activity-heatmap";
import type { DailyStat, PublicProfilePermissions, PublicUser, SliceTotal, Stats, StatsRange } from "@/lib/api";

export type RangeOption = { value: StatsRange; label: string };

export type ProfileViewProps = {
  user: PublicUser;
  username: string;
  stats?: Stats;
  permissions: PublicProfilePermissions;
  languageColors: Record<string, string>;
  range: StatsRange;
  setRange: (range: StatsRange) => void;
  ranges: RangeOption[];
};

export function ProfileView(props: ProfileViewProps) {
  switch (props.user.layout) {
    case "spotlight":
      return <SpotlightLayout {...props} />;
    case "rail":
      return <RailLayout {...props} />;
    default:
      return <TerminalLayout {...props} />;
  }
}

/* ------------------------------- shared bits ------------------------------ */

function displayName(user: PublicUser, username: string) {
  return user.name || user.github_username || username;
}

function initials(name: string) {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((part) => part[0]?.toUpperCase() ?? "").join("") || "·";
}

function Avatar({ user, username, size = 64, square = false }: { user: PublicUser; username: string; size?: number; square?: boolean }) {
  const radius = square ? "rounded-lg" : "rounded-full";
  if (user.avatar_url) {
    // eslint-disable-next-line @next/next/no-img-element
    return <img src={user.avatar_url} alt={displayName(user, username)} width={size} height={size} className={`${radius} border border-line object-cover`} style={{ width: size, height: size }} />;
  }
  return (
    <div className={`grid place-items-center border border-line bg-panel font-semibold text-accent ${radius}`} style={{ width: size, height: size, fontSize: size / 2.6 }}>
      {initials(displayName(user, username))}
    </div>
  );
}

function RangeTabs({ range, setRange, ranges }: Pick<ProfileViewProps, "range" | "setRange" | "ranges">) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {ranges.map((item) => (
        <button
          key={item.value}
          onClick={() => setRange(item.value)}
          className={`rounded px-2.5 py-1 text-xs transition ${range === item.value ? "bg-accent text-ink" : "border border-line text-zinc-400 hover:bg-white/5"}`}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

function SocialLinks({ user, className = "" }: { user: PublicUser; className?: string }) {
  const links: Array<{ href: string; label: string; icon: ReactNode }> = [];
  if (user.github_url) links.push({ href: user.github_url, label: "GitHub", icon: <Github size={16} /> });
  if (user.website_url) links.push({ href: user.website_url, label: "Website", icon: <Globe size={16} /> });
  if (user.twitter_url) links.push({ href: user.twitter_url, label: "Twitter", icon: <Twitter size={16} /> });
  if (user.linkedin_url) links.push({ href: user.linkedin_url, label: "LinkedIn", icon: <Linkedin size={16} /> });
  if (user.mastodon_url) links.push({ href: user.mastodon_url, label: "Mastodon", icon: <Globe size={16} /> });
  if (user.email) links.push({ href: `mailto:${user.email}`, label: user.email, icon: <Mail size={16} /> });
  if (links.length === 0) return null;
  return (
    <div className={`flex flex-wrap items-center gap-2 ${className}`}>
      {links.map((link) => (
        <a
          key={link.label}
          href={link.href}
          target={link.href.startsWith("mailto:") ? undefined : "_blank"}
          rel="noreferrer"
          title={link.label}
          className="inline-grid h-8 w-8 place-items-center rounded border border-line text-zinc-400 transition hover:border-accent hover:text-accent"
        >
          {link.icon}
        </a>
      ))}
    </div>
  );
}

function MetaChips({ user, className = "text-zinc-400" }: { user: PublicUser; className?: string }) {
  const chips: ReactNode[] = [];
  const place = user.location || user.country;
  if (place) chips.push(<span key="loc" className="inline-flex items-center gap-1"><MapPin size={13} /> {place}</span>);
  if (user.pronouns) chips.push(<span key="pro">{user.pronouns}</span>);
  const work = [user.role, user.company].filter(Boolean).join(" · ");
  if (work) chips.push(<span key="work" className="inline-flex items-center gap-1"><Briefcase size={13} /> {work}</span>);
  if (chips.length === 0) return null;
  return (
    <div className={`flex flex-wrap items-center gap-x-3 gap-y-1 text-sm ${className}`}>
      {chips.map((chip, index) => (
        <span key={index} className="inline-flex items-center">{chip}</span>
      ))}
    </div>
  );
}

function HireBadge({ user }: { user: PublicUser }) {
  if (!user.available_for_hire) return null;
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-400/30 bg-emerald-500/10 px-2.5 py-1 text-xs font-medium text-emerald-300">
      <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" /> Available for hire
    </span>
  );
}

const WEEKDAY_LABELS = ["", "Mon", "", "Wed", "", "Fri", ""];
const MONTH_LABELS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// ContributionGraph renders the daily activity as a GitHub-style calendar:
// one column per week, seven weekday rows, intensity by coding time.
function ContributionGraph({ days }: { days: DailyStat[] }) {
  if (days.length === 0) return null;
  const max = days.reduce((acc, day) => Math.max(acc, day.total_seconds), 0);
  const lead = new Date(days[0].date).getUTCDay();
  const cells: Array<DailyStat | null> = [...Array.from({ length: lead }, () => null), ...days];
  while (cells.length % 7 !== 0) cells.push(null);
  const weeks: Array<Array<DailyStat | null>> = [];
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
  const monthLabels = weeks.map((week) => {
    const firstReal = week.find((cell): cell is DailyStat => Boolean(cell));
    if (!firstReal) return "";
    const date = new Date(firstReal.date);
    return date.getUTCDate() <= 7 ? MONTH_LABELS[date.getUTCMonth()] : "";
  });
  return (
    <div className="overflow-x-auto">
      <div className="inline-flex flex-col gap-1">
        <div className="flex gap-[3px] pl-[30px] text-[10px] text-zinc-600">
          {weeks.map((_, index) => (
            <span key={index} className="w-3 shrink-0">{monthLabels[index]}</span>
          ))}
        </div>
        <div className="flex gap-[3px]">
          <div className="flex w-[27px] flex-col gap-[3px] text-[9px] leading-3 text-zinc-600">
            {WEEKDAY_LABELS.map((label, index) => (
              <span key={index} className="h-3">{label}</span>
            ))}
          </div>
          {weeks.map((week, weekIndex) => (
            <div key={weekIndex} className="flex flex-col gap-[3px]">
              {week.map((cell, dayIndex) =>
                cell ? (
                  <span key={dayIndex} title={activityHeatmapTitle(cell)} className={`h-3 w-3 rounded-[2px] border ${activityHeatmapClass(cell, max)}`} />
                ) : (
                  <span key={dayIndex} className="h-3 w-3" />
                )
              )}
            </div>
          ))}
        </div>
        <div className="flex items-center justify-end gap-1 pt-1 text-[10px] text-zinc-600">
          <span>Less</span>
          {[0, 1, 2, 3, 4].map((level) => (
            <span key={level} className={`h-3 w-3 rounded-[2px] border ${activityHeatmapClass({ total_seconds: level === 0 ? 0 : (max * level) / 4 }, max)}`} />
          ))}
          <span>More</span>
        </div>
      </div>
    </div>
  );
}

function LangBars({ rows, colors, limit = 6 }: { rows: SliceTotal[]; colors: Record<string, string>; limit?: number }) {
  const top = rows.slice(0, limit);
  const max = top.reduce((acc, row) => Math.max(acc, row.total_seconds), 0) || 1;
  if (top.length === 0) return <p className="text-sm text-zinc-600">No data yet.</p>;
  return (
    <div className="grid gap-2">
      {top.map((row) => (
        <div key={row.name} className="grid grid-cols-[7rem_1fr_auto] items-center gap-3 text-sm">
          <span className="truncate text-zinc-300">{row.name}</span>
          <span className="h-2 overflow-hidden rounded-full bg-white/5">
            <span
              className="block h-full rounded-full"
              style={{ width: `${Math.max(4, (row.total_seconds / max) * 100)}%`, background: colors[row.name] ?? "#00b4d8" }}
            />
          </span>
          <span className="tabular-nums text-xs text-zinc-500">{row.text}</span>
        </div>
      ))}
    </div>
  );
}

function StintFooter() {
  return (
    <footer className="mt-10 border-t border-line pt-5 text-center text-xs text-zinc-600">
      Public profile · powered by Stint
    </footer>
  );
}

/* ------------------------------- terminal --------------------------------- */

function TerminalLayout({ user, username, stats, permissions, languageColors, range, setRange, ranges }: ProfileViewProps) {
  const name = displayName(user, username);
  return (
    <main className="min-h-screen px-4 py-8 font-mono lg:px-8">
      <div className="mx-auto max-w-5xl">
        <div className="overflow-hidden rounded-lg border border-line bg-panel">
          <div className="flex items-center gap-2 border-b border-line bg-ink/60 px-4 py-2.5">
            <span className="h-3 w-3 rounded-full bg-red-400/70" />
            <span className="h-3 w-3 rounded-full bg-amber-400/70" />
            <span className="h-3 w-3 rounded-full bg-emerald-400/70" />
            <span className="ml-2 text-xs text-zinc-500">~/{username}</span>
            <span className="ml-auto"><RangeTabs range={range} setRange={setRange} ranges={ranges} /></span>
          </div>
          <div className="p-5 sm:p-6">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
              <Avatar user={user} username={username} size={72} square />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <h1 className="text-2xl font-semibold tracking-tight text-zinc-50">{name}</h1>
                  <span className="text-sm text-accent">@{username}</span>
                  <HireBadge user={user} />
                </div>
                <div className="mt-2"><MetaChips user={user} /></div>
                {user.bio ? <p className="mt-3 max-w-2xl text-sm leading-6 text-zinc-400">{user.bio}</p> : null}
                <SocialLinks user={user} className="mt-3" />
              </div>
            </div>
          </div>
        </div>

        {permissions.total_time ? (
          <div className="mt-4 grid grid-cols-2 gap-3 lg:grid-cols-3">
            <TerminalStat label="total" value={stats?.human_readable_total ?? "0s"} />
            <TerminalStat label="daily avg" value={stats?.human_readable_daily_average ?? "0s"} />
            <TerminalStat label="best day" value={stats?.best_day.text ?? "0s"} hint={stats?.best_day.date} />
          </div>
        ) : null}

        {permissions.total_time && (stats?.days.length ?? 0) > 0 ? (
          <section className="mt-4 rounded-lg border border-line bg-panel p-5">
            <div className="mb-3 text-xs uppercase tracking-[0.16em] text-zinc-500">activity · {range.replace(/_/g, " ")}</div>
            <ContributionGraph days={stats?.days ?? []} />
          </section>
        ) : null}

        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          {permissions.languages ? (
            <section className="rounded-lg border border-line bg-panel p-5">
              <div className="mb-3 text-xs uppercase tracking-[0.16em] text-zinc-500">languages</div>
              <LangBars rows={stats?.languages ?? []} colors={languageColors} />
            </section>
          ) : null}
          {permissions.editors ? (
            <section className="rounded-lg border border-line bg-panel p-5">
              <div className="mb-3 text-xs uppercase tracking-[0.16em] text-zinc-500">editors</div>
              <LangBars rows={stats?.editors ?? []} colors={{}} />
            </section>
          ) : null}
          {permissions.projects ? (
            <section className="rounded-lg border border-line bg-panel p-5">
              <div className="mb-3 text-xs uppercase tracking-[0.16em] text-zinc-500">projects</div>
              <LangBars rows={stats?.projects ?? []} colors={{}} />
            </section>
          ) : null}
        </div>

        {permissions.ai ? <div className="mt-4"><AIPanel metrics={stats?.ai} /></div> : null}
        <StintFooter />
      </div>
    </main>
  );
}

function TerminalStat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-lg border border-line bg-panel px-4 py-3">
      <div className="text-[11px] uppercase tracking-[0.16em] text-zinc-500">{label}</div>
      <div className="mt-1 text-xl font-semibold text-zinc-50">{value}</div>
      {hint ? <div className="text-xs text-zinc-600">{hint}</div> : null}
    </div>
  );
}

/* ------------------------------- spotlight -------------------------------- */

function SpotlightLayout({ user, username, stats, permissions, languageColors, range, setRange, ranges }: ProfileViewProps) {
  const name = displayName(user, username);
  return (
    <main className="min-h-screen">
      <div className="relative h-24 bg-gradient-to-r from-accent/30 via-indigo-500/20 to-fuchsia-500/20 sm:h-28">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_20%_120%,rgba(0,180,216,0.4),transparent_55%)]" />
      </div>
      <div className="mx-auto max-w-3xl px-5">
        <div className="-mt-10 flex items-end gap-4">
          <div className="rounded-full ring-4 ring-ink">
            <Avatar user={user} username={username} size={84} />
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pb-1">
            <h1 className="text-2xl font-semibold tracking-tight">{name}</h1>
            <span className="text-sm text-accent">@{username}</span>
            <HireBadge user={user} />
          </div>
        </div>
        <div className="mt-3"><MetaChips user={user} /></div>
        {user.bio ? <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-400">{user.bio}</p> : null}
        <SocialLinks user={user} className="mt-3" />

        {permissions.total_time ? (
          <section className="mt-5 flex flex-col gap-4 rounded-xl border border-line bg-panel p-5 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <div className="text-4xl font-semibold tracking-tight text-zinc-50">{stats?.human_readable_total ?? "0 secs"}</div>
              <div className="mt-1 text-xs text-zinc-500">coding time</div>
            </div>
            <div className="flex items-center gap-6 sm:border-l sm:border-line sm:pl-6">
              <div>
                <div className="text-lg font-semibold text-zinc-100">{stats?.human_readable_daily_average ?? "0 secs"}</div>
                <div className="text-xs text-zinc-500">daily avg</div>
              </div>
              <div>
                <div className="text-lg font-semibold text-zinc-100">{stats?.best_day.text ?? "0 secs"}</div>
                <div className="text-xs text-zinc-500">best day</div>
              </div>
            </div>
          </section>
        ) : null}

        <div className="mt-3 flex justify-end"><RangeTabs range={range} setRange={setRange} ranges={ranges} /></div>

        {permissions.total_time && (stats?.days.length ?? 0) > 0 ? (
          <section className="mt-2 rounded-xl border border-line bg-panel p-5">
            <ContributionGraph days={stats?.days ?? []} />
          </section>
        ) : null}

        <div className="mt-4 grid gap-4 md:grid-cols-2">
          {permissions.languages ? <SliceDonut title="Languages" rows={stats?.languages ?? []} colors={languageColors} /> : null}
          {permissions.projects ? <SliceDonut title="Projects" rows={stats?.projects ?? []} /> : null}
          {permissions.editors ? <SliceDonut title="Editors" rows={stats?.editors ?? []} /> : null}
          {permissions.categories ? <SliceDonut title="Categories" rows={stats?.categories ?? []} /> : null}
        </div>

        {permissions.ai ? <div className="mt-4"><AIPanel metrics={stats?.ai} /></div> : null}
        <StintFooter />
      </div>
    </main>
  );
}

/* --------------------------------- rail ----------------------------------- */

function RailLayout({ user, username, stats, permissions, languageColors, range, setRange, ranges }: ProfileViewProps) {
  const name = displayName(user, username);
  return (
    <main className="min-h-screen">
      <div className="mx-auto max-w-6xl gap-6 px-5 py-8 lg:grid lg:grid-cols-[300px_1fr] lg:px-8">
        <aside className="lg:sticky lg:top-8 lg:self-start">
          <div className="rounded-2xl border border-line bg-panel p-6">
            <Avatar user={user} username={username} size={96} />
            <h1 className="mt-4 text-2xl font-semibold tracking-tight">{name}</h1>
            <p className="text-sm text-accent">@{username}</p>
            <div className="mt-3"><HireBadge user={user} /></div>
            {user.bio ? <p className="mt-4 text-sm leading-6 text-zinc-400">{user.bio}</p> : null}
            <div className="mt-4 space-y-2 text-sm text-zinc-400">
              {(user.location || user.country) ? <div className="flex items-center gap-2"><MapPin size={14} /> {user.location || user.country}</div> : null}
              {user.pronouns ? <div className="flex items-center gap-2"><span className="text-zinc-600">·</span> {user.pronouns}</div> : null}
              {(user.role || user.company) ? <div className="flex items-center gap-2"><Briefcase size={14} /> {[user.role, user.company].filter(Boolean).join(" · ")}</div> : null}
            </div>
            <SocialLinks user={user} className="mt-4" />
            {permissions.total_time ? (
              <div className="mt-6 space-y-3 border-t border-line pt-5">
                <RailTotal label="Total" value={stats?.human_readable_total ?? "0 secs"} />
                <RailTotal label="Daily average" value={stats?.human_readable_daily_average ?? "0 secs"} />
                <RailTotal label="Best day" value={stats?.best_day.text ?? "0 secs"} hint={stats?.best_day.date} />
              </div>
            ) : null}
          </div>
        </aside>

        <div className="mt-6 lg:mt-0">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-sm uppercase tracking-[0.16em] text-zinc-500">Activity</h2>
            <RangeTabs range={range} setRange={setRange} ranges={ranges} />
          </div>
          {permissions.total_time && (stats?.days.length ?? 0) > 0 ? (
            <section className="rounded-2xl border border-line bg-panel p-5">
              <ContributionGraph days={stats?.days ?? []} />
            </section>
          ) : null}
          <div className="mt-5 grid gap-5 xl:grid-cols-2">
            {permissions.languages ? (
              <section className="rounded-2xl border border-line bg-panel p-5">
                <div className="mb-3 text-sm font-medium text-zinc-200">Languages</div>
                <LangBars rows={stats?.languages ?? []} colors={languageColors} />
              </section>
            ) : null}
            {permissions.projects ? (
              <section className="rounded-2xl border border-line bg-panel p-5">
                <div className="mb-3 text-sm font-medium text-zinc-200">Projects</div>
                <LangBars rows={stats?.projects ?? []} colors={{}} />
              </section>
            ) : null}
          </div>
          {permissions.ai ? <div className="mt-5"><AIPanel metrics={stats?.ai} /></div> : null}
          <StintFooter />
        </div>
      </div>
    </main>
  );
}

function RailTotal({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-sm text-zinc-500">{label}</span>
      <span className="text-right">
        <span className="block font-semibold text-zinc-50">{value}</span>
        {hint ? <span className="block text-xs text-zinc-600">{hint}</span> : null}
      </span>
    </div>
  );
}
