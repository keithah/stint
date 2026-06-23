export type User = {
  id: string;
  github_username: string;
  email?: string;
  full_name?: string;
  avatar_url?: string;
  country?: string;
  timezone: string;
  timeout_minutes: number;
  writes_only: boolean;
  has_public_profile: boolean;
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

export type PublicProjectVisibility = "none" | "public_repos" | "all";

export type ProfileLayout = "terminal" | "spotlight" | "rail";
export type ProfileVisibility = "public" | "private";

export type PublicProfileFields = {
  bio?: string;
  location?: string;
  website_url?: string;
  twitter_username?: string;
  linkedin_url?: string;
  mastodon_url?: string;
  pronouns?: string;
  company?: string;
  role?: string;
  layout?: ProfileLayout;
  available_for_hire?: boolean;
  email_public?: boolean;
  visibility?: Record<string, ProfileVisibility>;
};

export type PublicProfilePermissions = {
  total_time: boolean;
  projects: boolean;
  project_visibility: PublicProjectVisibility;
  languages: boolean;
  editors: boolean;
  machines: boolean;
  operating_systems: boolean;
  categories: boolean;
  ai: boolean;
  summaries: boolean;
  github: boolean;
};

export type ServerMeta = {
  api_url: string;
  base_url: string;
  hostname: string;
  ip: string;
  version: string;
};

export type PublicUser = {
  id: string;
  username: string;
  name?: string;
  github_username?: string;
  github_url?: string;
  avatar_url?: string;
  layout?: ProfileLayout;
  bio?: string;
  location?: string;
  country?: string;
  website_url?: string;
  twitter_username?: string;
  twitter_url?: string;
  linkedin_url?: string;
  mastodon_url?: string;
  pronouns?: string;
  company?: string;
  role?: string;
  available_for_hire?: boolean;
  email?: string;
  permissions: PublicProfilePermissions;
};

export type SliceTotal = {
  name: string;
  total_seconds: number;
  text: string;
};

export type DailyStat = {
  date: string;
  total_seconds: number;
  text: string;
  projects?: SliceTotal[];
};

export type WeekdayStat = {
  name: string;
  day: number;
  total_seconds: number;
  text: string;
  active_days: number;
  average_seconds: number;
  average_text: string;
};

export type DailyAverageTrendStat = {
  date: string;
  total_seconds: number;
  text: string;
  average_seconds: number;
  average_text: string;
  day_count: number;
};

export type HourlyStat = {
  hour: number;
  label: string;
  total_seconds: number;
  text: string;
  projects?: SliceTotal[];
  languages?: SliceTotal[];
};

export type AIStat = {
  name: string;
  ai_seconds: number;
  ai_line_changes: number;
  human_line_changes: number;
  ai_input_tokens: number;
  ai_output_tokens: number;
  ai_prompt_length: number;
  session_count: number;
  estimated_cost_cents: number;
};

export type AICostPeriod = {
  agent: string;
  daily_cents: number;
  weekly_cents: number;
  monthly_cents: number;
  total_cents: number;
};

export type AIMetrics = {
  ai_line_changes: number;
  human_line_changes: number;
  ai_percentage: number;
  human_review_percentage: number;
  follow_up_edits: number;
  ai_input_tokens: number;
  ai_output_tokens: number;
  ai_prompt_length: number;
  prompt_count: number;
  average_prompt_length: number;
  median_prompt_length: number;
  session_count: number;
  estimated_cost_cents: number;
  agents: AIStat[];
  days: AIStat[];
  costs: AICostPeriod[];
};

export type Stats = {
  range: string;
  total_seconds: number;
  human_readable_total: string;
  daily_average_seconds: number;
  human_readable_daily_average: string;
  best_day: DailyStat;
  days: DailyStat[];
  hourly: HourlyStat[];
  projects: SliceTotal[];
  languages: SliceTotal[];
  editors: SliceTotal[];
  operating_systems: SliceTotal[];
  machines: SliceTotal[];
  categories: SliceTotal[];
  branches: SliceTotal[];
  dependencies: SliceTotal[];
  ai: AIMetrics;
  project_ai: AIStat[];
  is_up_to_date: boolean;
  percent_calculated: number;
};

type CalendarYearRange = `${number}`;
type CalendarMonthRange = `${number}-${number}`;

export type StatsRange = "last_7_days" | "last_30_days" | "last_6_months" | "last_year" | "all_time" | CalendarYearRange | CalendarMonthRange;

export type StatusBarStats = {
  total_seconds: number;
  grand_total_text: string;
  project?: string;
  project_seconds?: number;
  project_text?: string;
  language?: string;
  language_seconds?: number;
  language_text?: string;
  range: string;
  percent_calculated: number;
};

export type Project = {
  id: string;
  name: string;
  color?: string;
  has_public_url: boolean;
  badge?: string;
  first_heartbeat_at?: string;
  last_heartbeat_at?: string;
  created_at: string;
};

export type ProgramLanguage = {
  name: string;
  color: string;
};

export type EditorMetadata = {
  name: string;
  key: string;
  version?: string;
};

export type Goal = {
  id: string;
  title: string;
  custom_title?: string;
  delta: "day" | "week";
  seconds: number;
  languages?: string[];
  editors?: string[];
  projects?: string[];
  ignore_days?: string[];
  ignore_zero_days: boolean;
  improve_by_percent?: number;
  is_enabled: boolean;
  is_inverse: boolean;
  is_snoozed: boolean;
  snooze_until?: string;
  created_at?: string;
  modified_at?: string;
};

export type GoalPayload = Omit<Goal, "id" | "created_at" | "modified_at">;

export type GoalProgress = {
  goal: Goal;
  actual_seconds: number;
  target_seconds: number;
  percent: number;
  is_complete: boolean;
  human_readable_actual: string;
  human_readable_target: string;
  remaining_seconds: number;
  is_snoozed: boolean;
  is_ignored: boolean;
};

export type AllTime = {
  total_seconds: number;
  text: string;
  stats: Stats;
};

export type ProjectDetail = {
  project: Project;
  stats: Stats;
};

export type CommitSummary = {
  id: string;
  hash: string;
  truncated_hash: string;
  branch?: string;
  ref?: string;
  total_seconds: number;
  human_readable_total: string;
  human_readable_total_with_seconds: string;
  created_at?: string;
  author_date?: string;
  committer_date?: string;
  html_url?: string;
  url?: string;
};

export type ProjectCommitsResponse = {
  commits: CommitSummary[];
  branch?: string;
  page: number;
  next_page?: number | null;
  next_page_url?: string | null;
  prev_page?: number | null;
  prev_page_url?: string | null;
  total: number;
  total_pages: number;
  status: string;
  project: {
    id: string;
    name: string;
    color?: string;
    has_public_url: boolean;
    badge?: string;
    privacy: string;
    repository?: string | null;
  };
};

export type Heartbeat = {
  id: string;
  entity: string;
  type: string;
  category?: string;
  time: number;
  project?: string;
  language?: string;
  editor?: string;
  operating_system?: string;
  ai_line_changes?: number;
  human_line_changes?: number;
  ai_session?: string;
  ai_input_tokens?: number;
  ai_output_tokens?: number;
  ai_model?: string;
  ai_agent?: string;
  ai_agent_version?: string;
  ai_agent_complexity?: string;
};

export type DurationSlice = "project" | "language" | "editor" | "operating_system" | "machine" | "category" | "branch" | "dependencies";

export type Duration = {
  name: string;
  project?: string;
  language?: string;
  time: number;
  duration: number;
};

export type FileExpert = {
  total: {
    decimal: string;
    digital: string;
    text: string;
    total_seconds: number;
  };
  user: {
    id: string;
    is_current_user: boolean;
    long_name: string;
    name: string;
  };
};

export type ExternalDuration = {
  id: string;
  external_id: string;
  provider: string;
  entity: string;
  type: string;
  category?: string;
  start_time: number;
  end_time: number;
  project?: string;
  branch?: string;
  language?: string;
  meta?: string;
  created_at: string;
};

export type ExternalDurationBulkResponse = {
  responses: Array<{
    status: number;
    data?: ExternalDuration;
    error?: string;
  }>;
};

export type SummaryDay = {
  range: {
    date: string;
    start: string;
    end: string;
  };
  grand_total: {
    total_seconds: number;
    text: string;
  };
  projects: SliceTotal[];
  languages: SliceTotal[];
  categories?: SliceTotal[];
  dependencies?: SliceTotal[];
  editors?: SliceTotal[];
  machines?: SliceTotal[];
  operating_systems?: SliceTotal[];
};

export type Leaderboard = {
  id: string;
  name: string;
  time_range: StatsRange;
  created_at: string;
  modified_at?: string;
};

export type LeaderboardEntry = {
  user_id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
  country?: string;
  total_seconds: number;
  text: string;
  rank: number;
};

export type LeaderboardMember = {
  user_id: string;
  username: string;
  full_name?: string;
  role: "owner" | "member";
};

export type DataDump = {
  id: string;
  type: "heartbeats" | "daily";
  status: string;
  percent_complete: number;
  download_url?: string;
  expires_at?: string;
  created_at: string;
};

export type CustomRuleDestination = {
  destination: string;
  destination_value: string;
};

export type CustomRule = {
  id?: string;
  action: "change" | "delete";
  source: string;
  operation: string;
  source_value: string;
  priority: number;
  destinations?: CustomRuleDestination[];
};

export type CustomRulesProgress = {
  status: string;
  percent_complete: number;
  total: number;
  changed: number;
  deleted: number;
  error?: string;
  started_at?: string;
  completed_at?: string;
  modified_at: string;
};

export type MachineName = {
  id: string;
  name: string;
  value?: string;
  timezone?: string;
  last_seen_at?: string;
  created_at: string;
};

export type UserAgent = {
  id: string;
  value: string;
  editor: string;
  ai_model?: string;
  ai_provider?: string;
  ai_agent?: string;
  ai_agent_version?: string;
  ai_agent_complexity?: string;
  version?: string;
  os?: string;
  last_seen_at: string;
  is_browser_extension: boolean;
  is_desktop_app: boolean;
  created_at: string;
};

export type APIKey = {
  id: string;
  name: string;
  fingerprint: string;
  scopes: string[];
  last_used_at?: string;
  created_at: string;
};

export type OAuthApp = {
  id: string;
  name: string;
  client_id: string;
  client_secret?: string;
  client_secret_fingerprint?: string;
  redirect_uris: string[];
  scopes: string[];
  created_at: string;
  modified_at: string;
};

export type ShareToken = {
  id: string;
  name: string;
  token?: string;
  fingerprint: string;
  last_used_at?: string;
  created_at: string;
};

export type ImportResult = {
  status: string;
  inserted: number;
  duplicates: number;
  invalid: number;
  total: number;
};

export type AICostSetting = {
  agent: string;
  input_cost_per_million_cents: number;
  output_cost_per_million_cents: number;
  modified_at?: string;
};

export type CustomPricing = {
  model: string;
  input_per_million_usd: number;
  output_per_million_usd: number;
  cache_write_per_million_usd: number;
  cache_read_per_million_usd: number;
  created_at?: string;
  modified_at?: string;
};

export type UsageCostMode = "auto" | "calculate" | "display";

export type UsageTotal = {
  cost_usd: number;
  marginal_usd: number;
  event_count: number;
  input_tokens: number;
  output_tokens: number;
  cache_create_tokens: number;
  cache_read_tokens: number;
  reasoning_tokens: number;
};

export type UsageSlice = {
  name: string;
  cost_usd: number;
  marginal_usd: number;
  tokens: number;
  event_count: number;
};

export type UsageDay = {
  date: string;
  cost_usd: number;
  marginal_usd: number;
  tokens: number;
};

export type UsageSummary = {
  range: string;
  cost_mode: UsageCostMode;
  total: UsageTotal;
  by_agent: UsageSlice[];
  by_model: UsageSlice[];
  by_project: UsageSlice[];
  by_day: UsageDay[];
  unpriced_models: string[];
};

export type UsageBlock = {
  start: string;
  end: string;
  is_active: boolean;
  cost_usd: number;
  tokens: number;
  event_count: number;
};

export type UsageCurrentBlock = {
  start: string;
  end: string;
  is_active: boolean;
  elapsed_minutes: number;
  cost_usd: number;
  tokens: number;
  burn_rate_cost_per_hour: number;
  burn_rate_tokens_per_min: number;
  projected_block_cost_usd: number;
  projected_day_cost_usd: number;
  projected_month_cost_usd: number;
};

export type UsageBlocks = {
  cost_mode: UsageCostMode;
  blocks: UsageBlock[];
  current: UsageCurrentBlock | null;
};

export type UsageEvent = {
  id: string;
  timestamp: string;
  agent?: string;
  model?: string;
  project?: string;
  cost_usd: number;
  marginal_usd: number;
  input_tokens: number;
  output_tokens: number;
  cache_create_tokens: number;
  cache_read_tokens: number;
  reasoning_tokens: number;
};

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

export function wakatimeAPIURL() {
  const base = apiBase || (typeof window !== "undefined" ? window.location.origin : "");
  return `${base.replace(/\/$/, "")}/api/v1`;
}

export function dataDumpDownloadURL(path?: string | null) {
  if (!path) {
    return "#";
  }
  if (/^https?:\/\//i.test(path) || path.startsWith("#")) {
    return path;
  }
  if (!apiBase) {
    return path.startsWith("/") ? path : `/${path}`;
  }
  return new URL(path, `${apiBase.replace(/\/$/, "")}/`).toString();
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {})
    }
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    const message = body.error ?? body.errors?.[0] ?? `Request failed with ${response.status}`;
    throw new Error(message);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json();
}

export async function me() {
  return request<{ data: User }>("/api/v1/auth/me");
}

export async function logout() {
  await request("/auth/logout", { method: "POST" });
}

export async function serverMeta() {
  return request<{ data: ServerMeta }>("/api/v1/meta");
}

export async function seedDevKey() {
  return request<{ data: { user: User; api_key: string } }>("/api/v1/dev/seed-key", { method: "POST" });
}

export async function statsLast7Days() {
  return request<{ data: Stats }>("/api/v1/users/current/stats/last_7_days");
}

export async function statsForRange(range: StatsRange) {
  return request<{ data: Stats }>(`/api/v1/users/current/stats/${range}`);
}

export async function statusBarToday() {
  return request<{ data: StatusBarStats }>("/api/v1/users/current/status_bar/today");
}

export async function allTimeSinceToday() {
  return request<{ data: AllTime }>("/api/v1/users/current/all_time_since_today");
}

export async function listProjects() {
  return request<{ data: Project[] }>("/api/v1/users/current/projects");
}

export async function listProgramLanguages() {
  return request<{ data: ProgramLanguage[] }>("/api/v1/program_languages");
}

export async function listEditors() {
  return request<{ data: EditorMetadata[] }>("/api/v1/editors");
}

export async function projectDetail(project: string, range?: StatsRange | string) {
  const query = range ? `?range=${encodeURIComponent(range)}` : "";
  return request<{ data: ProjectDetail }>(`/api/v1/users/current/projects/${encodeURIComponent(project)}${query}`);
}

export async function listProjectCommits(project: string, options: { branch?: string; page?: number } = {}) {
  const query = new URLSearchParams();
  if (options.branch) {
    query.set("branch", options.branch);
  }
  if (options.page && options.page > 1) {
    query.set("page", String(options.page));
  }
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<ProjectCommitsResponse>(`/api/v1/users/current/projects/${encodeURIComponent(project)}/commits${suffix}`);
}

export async function getProjectCommit(project: string, hash: string) {
  return request<{ commit: CommitSummary; branch?: string; project: ProjectCommitsResponse["project"]; status: string }>(
    `/api/v1/users/current/projects/${encodeURIComponent(project)}/commits/${encodeURIComponent(hash)}`
  );
}

export async function listMachines() {
  return request<{ data: MachineName[] }>("/api/v1/users/current/machine_names");
}

export async function listUserAgents() {
  return request<{ data: UserAgent[] }>("/api/v1/users/current/user_agents");
}

export async function listGoals() {
  return request<{ data: GoalProgress[] }>("/api/v1/users/current/goals");
}

export async function createGoal(goal: GoalPayload) {
  return request<{ data: Goal }>("/api/v1/users/current/goals", {
    method: "POST",
    body: JSON.stringify(goal)
  });
}

export async function getGoal(id: string) {
  return request<{ data: Goal }>(`/api/v1/users/current/goals/${encodeURIComponent(id)}`);
}

export async function updateGoal(id: string, goal: GoalPayload) {
  return request<{ data: Goal }>(`/api/v1/users/current/goals/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(goal)
  });
}

export async function deleteGoal(id: string) {
  await request(`/api/v1/users/current/goals/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function insight(type: string, range: StatsRange) {
  return request<{ data: Stats | SliceTotal[] | DailyStat[] | HourlyStat[] | WeekdayStat[] | DailyAverageTrendStat[] | AIStat[] | DailyStat | { seconds: number; text: string }; range: string }>(
    `/api/v1/users/current/insights/${encodeURIComponent(type)}/${encodeURIComponent(range)}`
  );
}

export async function heartbeatsForDay(date: string) {
  return request<{ data: Heartbeat[] }>(`/api/v1/users/current/heartbeats?date=${encodeURIComponent(date)}`);
}

export async function durationsForDay(date: string, sliceBy: DurationSlice = "project") {
  const query = new URLSearchParams({ date, slice_by: sliceBy });
  return request<{ data: Duration[] }>(`/api/v1/users/current/durations?${query.toString()}`);
}

export async function deleteHeartbeats(date: string, ids: string[]) {
  return request<{ data: { deleted: number } }>("/api/v1/users/current/heartbeats.bulk", {
    method: "DELETE",
    body: JSON.stringify({ date, ids })
  });
}

export async function fileExperts(entity: string, project?: string) {
  return request<{ data: FileExpert[] }>("/api/v1/users/current/file_experts", {
    method: "POST",
    body: JSON.stringify(project ? { entity, project } : { entity })
  });
}

export async function publicLeaders(language?: string, country?: string) {
  const query = new URLSearchParams();
  if (language) {
    query.set("language", language);
  }
  if (country) {
    query.set("country", country);
  }
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<{ data: LeaderboardEntry[]; meta: { cached: boolean; range: string; language?: string; country?: string } }>(`/api/v1/leaders${suffix}`);
}

export async function listLeaderboards() {
  return request<{ data: Leaderboard[] }>("/api/v1/users/current/leaderboards");
}

export async function createLeaderboard(name: string, timeRange: StatsRange) {
  return request<{ data: Leaderboard }>("/api/v1/users/current/leaderboards", {
    method: "POST",
    body: JSON.stringify({ name, time_range: timeRange })
  });
}

export async function leaderboardEntries(id: string) {
  return request<{ data: LeaderboardEntry[]; board: Leaderboard; members: LeaderboardMember[] }>(`/api/v1/users/current/leaderboards/${encodeURIComponent(id)}`);
}

export async function updateLeaderboard(id: string, name: string, timeRange: StatsRange) {
  return request<{ data: Leaderboard }>(`/api/v1/users/current/leaderboards/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify({ name, time_range: timeRange })
  });
}

export async function deleteLeaderboard(id: string) {
  await request(`/api/v1/users/current/leaderboards/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function addLeaderboardMember(id: string, username: string) {
  return request<{ data: { user_id: string; username: string; full_name?: string } }>(`/api/v1/users/current/leaderboards/${encodeURIComponent(id)}/members`, {
    method: "POST",
    body: JSON.stringify({ username })
  });
}

export async function removeLeaderboardMember(id: string, userID: string) {
  await request(`/api/v1/users/current/leaderboards/${encodeURIComponent(id)}/members/${encodeURIComponent(userID)}`, { method: "DELETE" });
}

export async function listExternalDurations() {
  return request<{ data: ExternalDuration[] }>("/api/v1/users/current/external_durations");
}

export async function createExternalDuration(duration: Omit<ExternalDuration, "id" | "created_at">) {
  return request<{ data: ExternalDuration }>("/api/v1/users/current/external_durations", {
    method: "POST",
    body: JSON.stringify(duration)
  });
}

export async function createExternalDurationsBulk(durations: Array<Omit<ExternalDuration, "id" | "created_at">>) {
  return request<ExternalDurationBulkResponse>("/api/v1/users/current/external_durations.bulk", {
    method: "POST",
    body: JSON.stringify(durations)
  });
}

export async function deleteExternalDurationsBulk(ids: string[]) {
  return request<{ data: { deleted: number } }>("/api/v1/users/current/external_durations.bulk", {
    method: "DELETE",
    body: JSON.stringify({ ids })
  });
}

export async function summaries(start: string, end: string) {
  return request<{ data: SummaryDay[] }>(`/api/v1/users/current/summaries?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`);
}

export async function listDataDumps() {
  return request<{ data: DataDump[] }>("/api/v1/users/current/data_dumps");
}

export async function createDataDump(type: "heartbeats" | "daily") {
  return request<{ data: DataDump }>("/api/v1/users/current/data_dumps", {
    method: "POST",
    body: JSON.stringify({ type })
  });
}

export async function listCustomRules() {
  return request<{ data: CustomRule[] }>("/api/v1/users/current/custom_rules");
}

export async function replaceCustomRules(rules: CustomRule[]) {
  return request<{ data: CustomRule[] }>("/api/v1/users/current/custom_rules", {
    method: "PUT",
    body: JSON.stringify(rules)
  });
}

export async function deleteCustomRule(id: string) {
  await request(`/api/v1/users/current/custom_rules/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function customRulesProgress() {
  return request<{ data: CustomRulesProgress }>("/api/v1/users/current/custom_rules_progress");
}

export async function abortCustomRulesProgress() {
  return request<{ data: CustomRulesProgress }>("/api/v1/users/current/custom_rules_progress", {
    method: "DELETE"
  });
}

export type UserUpdatePayload = Pick<
  User,
  | "timezone"
  | "timeout_minutes"
  | "writes_only"
  | "has_public_profile"
  | "country"
  | "heartbeat_retention_days"
  | "public_username"
  | "public_display_name"
  | "public_github_link_enabled"
  | "public_show_total_time"
  | "public_show_projects"
  | "public_project_visibility"
  | "public_show_languages"
  | "public_show_editors"
  | "public_show_machines"
  | "public_show_operating_systems"
  | "public_show_categories"
  | "public_show_ai"
  | "public_show_summaries"
  | "public_profile"
>;

export async function updateUser(payload: UserUpdatePayload) {
  return request<{ data: User }>("/api/v1/users/current", {
    method: "PUT",
    body: JSON.stringify(payload)
  });
}

export async function deleteCurrentUser(confirmation: string) {
  return request<{ data: { deleted: boolean } }>("/api/v1/users/current", {
    method: "DELETE",
    body: JSON.stringify({ confirmation })
  });
}

export async function listKeys() {
  return request<{ data: APIKey[] }>("/api/v1/api_keys");
}

export async function createKey(name: string, scopes?: string[]) {
  return request<{ data: { key: APIKey; api_key: string } }>("/api/v1/api_keys", {
    method: "POST",
    body: JSON.stringify({ name, scopes })
  });
}

export async function revokeKey(id: string) {
  await request(`/api/v1/api_keys/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listOAuthApps() {
  return request<{ data: OAuthApp[] }>("/api/v1/oauth/apps");
}

export async function createOAuthApp(payload: { name: string; redirect_uris: string[]; scopes: string[] }) {
  return request<{ data: OAuthApp }>("/api/v1/oauth/apps", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export async function deleteOAuthApp(id: string) {
  await request(`/api/v1/oauth/apps/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listShareTokens() {
  return request<{ data: ShareToken[] }>("/api/v1/users/current/share_tokens");
}

export async function createShareToken(name: string) {
  return request<{ data: ShareToken }>("/api/v1/users/current/share_tokens", {
    method: "POST",
    body: JSON.stringify({ name })
  });
}

export async function deleteShareToken(id: string) {
  await request(`/api/v1/users/current/share_tokens/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function publicShareStats(user: string, token: string, range: StatsRange) {
  return request<{ data: Stats; user: { id: string; username: string; name?: string } }>(
    `/api/v1/users/${encodeURIComponent(user)}/share/${encodeURIComponent(token)}/stats?range=${encodeURIComponent(range)}`
  );
}

export async function publicUserProfile(user: string) {
  return request<{ data: PublicUser }>(`/api/v1/users/${encodeURIComponent(user)}`);
}

export async function publicUserStats(user: string, range: StatsRange) {
  return request<{ data: Stats; user: PublicUser }>(`/api/v1/users/${encodeURIComponent(user)}/stats/${encodeURIComponent(range)}`);
}

export async function publicUserSummaries(user: string, start: string, end: string) {
  const query = new URLSearchParams({ start, end });
  return request<{ data: SummaryDay[]; user: PublicUser }>(`/api/v1/users/${encodeURIComponent(user)}/summaries?${query.toString()}`);
}

export async function publicShareStatsByToken(token: string, range: StatsRange) {
  return request<{ data: Stats; user: { id: string; username: string; name?: string } }>(`/api/v1/share/${encodeURIComponent(token)}/stats?range=${encodeURIComponent(range)}`);
}

export async function publicShareSummaries(user: string, token: string, start: string, end: string) {
  const query = new URLSearchParams({ start, end });
  return request<{ data: SummaryDay[]; user: { id: string; username: string; name?: string } }>(
    `/api/v1/users/${encodeURIComponent(user)}/share/${encodeURIComponent(token)}/summaries?${query.toString()}`
  );
}

export async function publicShareSummariesByToken(token: string, start: string, end: string) {
  const query = new URLSearchParams({ start, end });
  return request<{ data: SummaryDay[]; user: { id: string; username: string; name?: string } }>(`/api/v1/share/${encodeURIComponent(token)}/summaries?${query.toString()}`);
}

export async function importWakaTimeDump(file: File) {
  const form = new FormData();
  form.set("file", file);
  const response = await fetch(`${apiBase}/api/v1/users/current/imports/wakatime`, {
    method: "POST",
    credentials: "include",
    body: form
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    const message = body.error ?? body.errors?.[0] ?? `Request failed with ${response.status}`;
    throw new Error(message);
  }
  return response.json() as Promise<{ data: ImportResult }>;
}

export async function usageSummary(range: StatsRange, costMode: UsageCostMode = "auto") {
  const query = new URLSearchParams({ range, cost_mode: costMode });
  return request<{ data: UsageSummary }>(`/api/v1/users/current/usage_events/summary?${query.toString()}`);
}

export async function usageBlocks(range: StatsRange, costMode: UsageCostMode = "auto") {
  const query = new URLSearchParams({ range, cost_mode: costMode });
  return request<{ data: UsageBlocks }>(`/api/v1/users/current/usage_events/blocks?${query.toString()}`);
}

export async function usageExport(start: string, end: string) {
  const query = new URLSearchParams({ start, end });
  return request<{ data: UsageEvent[] }>(`/api/v1/users/current/usage_events?${query.toString()}`);
}

export async function listAICosts() {
  return request<{ data: AICostSetting[] }>("/api/v1/users/current/ai_costs");
}

export async function replaceAICosts(settings: AICostSetting[]) {
  return request<{ data: AICostSetting[] }>("/api/v1/users/current/ai_costs", {
    method: "PUT",
    body: JSON.stringify(settings)
  });
}

export async function listCustomPricing() {
  return request<{ data: CustomPricing[] }>("/api/v1/users/current/custom_pricing");
}

export async function upsertCustomPricing(pricing: CustomPricing) {
  return request<{ data: CustomPricing[] }>("/api/v1/users/current/custom_pricing", {
    method: "PUT",
    body: JSON.stringify(pricing)
  });
}

export async function deleteCustomPricing(model: string) {
  return request<void>(
    `/api/v1/users/current/custom_pricing/${encodeURIComponent(model)}`,
    { method: "DELETE" }
  );
}
