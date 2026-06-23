import type { PublicProfileFields, PublicProjectVisibility } from "@/lib/api";

export type PublicProfileDraft = {
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
  public_profile: PublicProfileFields;
};

export function Diagnostic({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded border border-line bg-ink p-3">
      <div className="text-xs uppercase tracking-[0.16em] text-zinc-500">{label}</div>
      <div className="mt-2 truncate text-sm text-zinc-200" title={value}>{value}</div>
    </div>
  );
}

export function PrivacyToggle({ label, detail, checked, onChange }: { label: string; detail: string; checked: boolean; onChange: (checked: boolean) => void }) {
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

export function ProfileField({
  label,
  placeholder,
  prefix,
  value,
  onChange,
  isPublic,
  onVisibility,
  textarea
}: {
  label: string;
  placeholder?: string;
  prefix?: string;
  value: string;
  onChange: (value: string) => void;
  isPublic: boolean;
  onVisibility: (isPublic: boolean) => void;
  textarea?: boolean;
}) {
  return (
    <label className="block">
      <span className="flex items-center justify-between gap-2 text-sm text-zinc-400">
        {label}
        <button
          type="button"
          onClick={() => onVisibility(!isPublic)}
          className={`rounded px-2 py-0.5 text-[10px] uppercase tracking-[0.12em] transition ${isPublic ? "bg-accent/15 text-accent" : "bg-white/5 text-zinc-500"}`}
        >
          {isPublic ? "Public" : "Private"}
        </button>
      </span>
      {textarea ? (
        <textarea
          className="mt-2 w-full rounded border border-line bg-panel px-3 py-2 text-sm outline-none focus:border-accent"
          rows={3}
          placeholder={placeholder}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <div className="mt-2 flex rounded border border-line bg-panel focus-within:border-accent">
          {prefix ? <span className="border-r border-line px-3 py-2 text-sm text-zinc-500">{prefix}</span> : null}
          <input
            className="min-w-0 flex-1 bg-transparent px-3 py-2 text-sm outline-none"
            placeholder={placeholder}
            value={value}
            onChange={(event) => onChange(event.target.value)}
          />
        </div>
      )}
    </label>
  );
}

export function noopSubscribe() {
  return () => {};
}

export function serverWakaTimeAPIURL() {
  return "/api/v1";
}

export function shareStatsJSONPURL(apiURL: string, token: string) {
  const query = new URLSearchParams({ range: "last_7_days", callback: "StintEmbed.render" });
  return `${apiURL}/share/${encodeURIComponent(token)}/stats?${query.toString()}`;
}

export function isHTTPURL(value: string) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}
