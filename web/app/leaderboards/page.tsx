"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LogIn, Plus, Trash2, Trophy, UserPlus, X } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/ui";
import { addLeaderboardMember, createLeaderboard, deleteLeaderboard, leaderboardEntries, listLeaderboards, me, publicLeaders, removeLeaderboardMember, updateLeaderboard, type LeaderboardEntry, type LeaderboardMember, type StatsRange } from "@/lib/api";
import { currentLeaderboardEntry, isCurrentLeaderboardUser } from "@/lib/leaderboard-current-user";
import { leaderboardRangeIsValid, normalizeLeaderboardRangeInput } from "@/lib/leaderboard-ranges";

const publicLanguageOptions = ["", "Go", "TypeScript", "JavaScript", "Python", "Rust", "Markdown"];
const publicCountryOptions = ["", "US", "CA", "GB", "DE", "FR", "IN", "BR", "AU", "JP"];

export default function LeaderboardsPage() {
  return (
    <AppShell>
      <LeaderboardsContent />
    </AppShell>
  );
}

function LeaderboardsContent() {
  const client = useQueryClient();
  const [name, setName] = useState("Local leaderboard");
  const [range, setRange] = useState<StatsRange>("last_7_days");
  const [customRange, setCustomRange] = useState("");
  const [editName, setEditName] = useState("");
  const [editRange, setEditRange] = useState<StatsRange | "">("");
  const [editCustomRange, setEditCustomRange] = useState("");
  const [activeBoardID, setActiveBoardID] = useState("");
  const [memberUsername, setMemberUsername] = useState("");
  const [publicLanguage, setPublicLanguage] = useState("");
  const [publicCountry, setPublicCountry] = useState("");
  const user = useQuery({ queryKey: ["me"], queryFn: me, retry: false });
  const isLoggedIn = Boolean(user.data?.data);
  const boards = useQuery({ queryKey: ["leaderboards"], queryFn: listLeaderboards, enabled: isLoggedIn, retry: false });
  const publicRows = useQuery({ queryKey: ["public-leaders", publicLanguage, publicCountry], queryFn: () => publicLeaders(publicLanguage || undefined, publicCountry || undefined), retry: false });
  const activeBoard = (boards.data?.data ?? []).find((board) => board.id === activeBoardID) ?? boards.data?.data[0];
  const privateRows = useQuery({ queryKey: ["leaderboard", activeBoard?.id], queryFn: () => leaderboardEntries(activeBoard?.id ?? ""), enabled: Boolean(activeBoard && isLoggedIn), retry: false });
  const publicLeaderboardRows = publicRows.data?.data ?? [];
  const currentPublicEntry = currentLeaderboardEntry(publicLeaderboardRows, user.data?.data.github_username);
  const createRange = normalizeLeaderboardRangeInput(customRange, range);
  const canCreateBoard = name.trim().length > 0 && Boolean(createRange);
  const activeBoardName = activeBoard?.name.trim() ?? "";
  const editedBoardName = editName.trim() || activeBoardName;
  const selectedEditRange = editRange || activeBoard?.time_range || "last_7_days";
  const saveRange = normalizeLeaderboardRangeInput(editCustomRange, selectedEditRange as StatsRange);
  const canSaveBoard = Boolean(activeBoard && editedBoardName && saveRange);

  const create = useMutation({
    mutationFn: () => createLeaderboard(name.trim(), createRange as StatsRange),
    onSuccess: (response) => {
      setActiveBoardID(response.data.id);
      setCustomRange("");
      client.invalidateQueries({ queryKey: ["leaderboards"] });
    }
  });
  const remove = useMutation({
    mutationFn: deleteLeaderboard,
    onSuccess: () => {
      setActiveBoardID("");
      client.invalidateQueries({ queryKey: ["leaderboards"] });
    }
  });
  const update = useMutation({
    mutationFn: () => updateLeaderboard(activeBoard?.id ?? "", editedBoardName, saveRange as StatsRange),
    onSuccess: () => {
      setEditCustomRange("");
      client.invalidateQueries({ queryKey: ["leaderboards"] });
      client.invalidateQueries({ queryKey: ["leaderboard", activeBoard?.id] });
    }
  });
  const addMember = useMutation({
    mutationFn: () => addLeaderboardMember(activeBoard?.id ?? "", memberUsername.trim()),
    onSuccess: () => {
      setMemberUsername("");
      client.invalidateQueries({ queryKey: ["leaderboard", activeBoard?.id] });
    }
  });
  const removeMember = useMutation({
    mutationFn: (userID: string) => removeLeaderboardMember(activeBoard?.id ?? "", userID),
    onSuccess: () => client.invalidateQueries({ queryKey: ["leaderboard", activeBoard?.id] })
  });
  const board = privateRows.data?.board ?? activeBoard;
  const members = privateRows.data?.members ?? [];

  return (
    <div className="mx-auto max-w-6xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<Trophy size={14} />}
        caption="Rankings"
        title="Leaderboards"
        sub="Public rankings are open to everyone. Sign in to create private boards for teams, friends, and local competitions."
      />

      {isLoggedIn ? (
        <section className="mb-5 rounded-md border border-line bg-panel p-5">
          <h2 className="font-medium">Create private leaderboard</h2>
          <div className="mt-4 grid gap-3 md:grid-cols-[1fr_180px_180px_auto]">
            <input className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={name} onChange={(event) => setName(event.target.value)} />
            <select className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={range} onChange={(event) => setRange(event.target.value as StatsRange)}>
              <option value="last_7_days">7 days</option>
              <option value="last_30_days">30 days</option>
              <option value="last_6_months">6 months</option>
              <option value="last_year">Year</option>
              <option value="all_time">All time</option>
            </select>
            <input
              aria-label="Create custom range"
              className={`rounded-md border bg-ink px-3 py-2 text-sm outline-none focus:border-accent ${customRange && !leaderboardRangeIsValid(customRange) ? "border-red-500/70" : "border-line"}`}
              placeholder="YYYY or YYYY-MM"
              value={customRange}
              onChange={(event) => setCustomRange(event.target.value)}
            />
            <button className="inline-flex items-center justify-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => create.mutate()} disabled={create.isPending || !canCreateBoard}>
              <Plus size={16} /> Create
            </button>
          </div>
        </section>
      ) : null}

      <section className={`grid gap-5 ${isLoggedIn ? "lg:grid-cols-2" : ""}`}>
        <RankingTable title="Public leaders" rows={publicLeaderboardRows} currentUsername={user.data?.data.github_username} currentRank={currentPublicEntry ? <CurrentRank entry={currentPublicEntry} /> : null}>
          <div className="flex flex-wrap gap-2">
            <select className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={publicLanguage} onChange={(event) => setPublicLanguage(event.target.value)}>
              {publicLanguageOptions.map((language) => (
                <option key={language || "all"} value={language}>
                  {language || "All languages"}
                </option>
              ))}
            </select>
            <select className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={publicCountry} onChange={(event) => setPublicCountry(event.target.value)}>
              {publicCountryOptions.map((country) => (
                <option key={country || "all"} value={country}>
                  {country || "All countries"}
                </option>
              ))}
            </select>
          </div>
        </RankingTable>
        {isLoggedIn ? (
        <div className="rounded-md border border-line bg-panel">
          <div className="flex items-center justify-between border-b border-line px-4 py-3">
            <div>
              <h2 className="font-medium">Private boards</h2>
              <p className="mt-1 text-sm text-zinc-500">{board ? `${board.name} · ${board.time_range.replace(/_/g, " ")}` : "No private leaderboard yet"}</p>
            </div>
            {activeBoard ? (
              <button className="rounded border border-red-900/70 p-2 text-red-300 hover:bg-red-950/40" onClick={() => remove.mutate(activeBoard.id)}>
                <Trash2 size={16} />
              </button>
            ) : null}
          </div>
          {(boards.data?.data.length ?? 0) > 0 ? (
            <div className="border-b border-line px-4 py-3">
              <select className="w-full rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={activeBoard?.id ?? ""} onChange={(event) => setActiveBoardID(event.target.value)}>
                {(boards.data?.data ?? []).map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </div>
          ) : null}
          {activeBoard ? (
            <div className="grid gap-2 border-b border-line px-4 py-3 md:grid-cols-[1fr_150px_170px_auto]">
              <input className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" placeholder={activeBoard.name} value={editName} onChange={(event) => setEditName(event.target.value)} />
              <select className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={editRange || activeBoard.time_range} onChange={(event) => setEditRange(event.target.value as StatsRange)}>
                <option value="last_7_days">7 days</option>
                <option value="last_30_days">30 days</option>
                <option value="last_6_months">6 months</option>
                <option value="last_year">Year</option>
                <option value="all_time">All time</option>
              </select>
              <input
                aria-label="Edit custom range"
                className={`rounded-md border bg-ink px-3 py-2 text-sm outline-none focus:border-accent ${editCustomRange && !leaderboardRangeIsValid(editCustomRange) ? "border-red-500/70" : "border-line"}`}
                placeholder="YYYY or YYYY-MM"
                value={editCustomRange}
                onChange={(event) => setEditCustomRange(event.target.value)}
              />
              <button className="rounded-md border border-line px-4 py-2 text-sm text-zinc-300 hover:bg-white/5 disabled:opacity-60" onClick={() => update.mutate()} disabled={update.isPending || !canSaveBoard}>
                Save
              </button>
            </div>
          ) : null}
          {activeBoard ? (
            <div className="border-b border-line px-4 py-3">
              <div className="grid gap-2 md:grid-cols-[1fr_auto]">
                <input className="rounded-md border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" placeholder="GitHub username" value={memberUsername} onChange={(event) => setMemberUsername(event.target.value)} />
                <button className="inline-flex items-center justify-center gap-2 rounded-md border border-line px-4 py-2 text-sm text-zinc-300 hover:bg-white/5" onClick={() => addMember.mutate()} disabled={addMember.isPending || !memberUsername.trim()}>
                  <UserPlus size={16} /> Add
                </button>
              </div>
              {addMember.isError ? <p className="mt-2 text-sm text-red-300">{addMember.error.message}</p> : null}
              <MemberRows members={members} onRemove={(userID) => removeMember.mutate(userID)} removing={removeMember.isPending} />
            </div>
          ) : null}
          <RankingRows rows={privateRows.data?.data ?? []} currentUsername={user.data?.data.github_username} />
        </div>
        ) : (
          <div className="rounded-md border border-line bg-panel p-5">
            <div className="mb-4 inline-flex h-10 w-10 items-center justify-center rounded-md border border-accent/30 bg-accent/10 text-accent">
              <LogIn size={18} />
            </div>
            <h2 className="font-medium">Private boards require a session</h2>
            <p className="mt-2 text-sm leading-6 text-zinc-400">Public leaders stay visible without login. Connect GitHub to create private leaderboards and add members.</p>
            <Link className="mt-6 inline-flex items-center gap-2 rounded-md bg-accent px-4 py-2 text-sm font-medium text-ink" href="/login">
              Login <LogIn size={15} />
            </Link>
          </div>
        )}
      </section>
    </div>
  );
}

function RankingTable({ title, rows, currentUsername, currentRank, children }: { title: string; rows: LeaderboardEntry[]; currentUsername?: string; currentRank?: ReactNode; children?: ReactNode }) {
  return (
    <div className="overflow-hidden rounded-md border border-line bg-panel/95 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
        <h2 className="font-medium">{title}</h2>
        {children}
      </div>
      {currentRank}
      <RankingRows rows={rows} currentUsername={currentUsername} />
    </div>
  );
}

function CurrentRank({ entry }: { entry: LeaderboardEntry }) {
  return (
    <div className="border-b border-accent/20 bg-accent/10 px-4 py-3">
      <div className="grid grid-cols-[76px_36px_1fr_100px] items-center gap-3 text-sm">
        <span className="text-xs font-medium uppercase tracking-[0.18em] text-accent">Your rank</span>
        <Avatar row={entry} />
        <span className="min-w-0">
          <span className="block truncate font-medium text-zinc-100">#{entry.rank} {entry.display_name || entry.username}</span>
          <span className="block truncate text-xs text-zinc-500">
            @{entry.username}
            {entry.country ? ` · ${entry.country}` : ""}
          </span>
        </span>
        <span className="text-right font-medium text-zinc-100">{entry.text}</span>
      </div>
    </div>
  );
}

function MemberRows({ members, onRemove, removing }: { members: LeaderboardMember[]; onRemove: (userID: string) => void; removing: boolean }) {
  return (
    <div className="mt-3 flex flex-wrap gap-2">
      {members.map((member) => (
        <span key={member.user_id} className="inline-flex items-center gap-2 rounded border border-line bg-ink px-2.5 py-1.5 text-xs text-zinc-300">
          {member.username}
          <span className="text-zinc-500">{member.role}</span>
          {member.role !== "owner" ? (
            <button className="text-zinc-500 hover:text-red-300" onClick={() => onRemove(member.user_id)} disabled={removing} aria-label={`Remove ${member.username}`}>
              <X size={14} />
            </button>
          ) : null}
        </span>
      ))}
      {members.length === 0 ? <span className="text-sm text-zinc-500">No members loaded.</span> : null}
    </div>
  );
}

function RankingRows({ rows, currentUsername }: { rows: LeaderboardEntry[]; currentUsername?: string }) {
  return (
    <div className="divide-y divide-line">
      {rows.map((row) => (
        <div key={`${row.rank}-${row.username}`} className={`grid grid-cols-[42px_36px_1fr_100px] items-center gap-3 px-4 py-3 text-sm transition hover:bg-white/[0.03] ${isCurrentLeaderboardUser(row, currentUsername) ? "bg-accent/10" : ""}`}>
          <span className="text-zinc-500">#{row.rank}</span>
          <Avatar row={row} />
          <span className="min-w-0">
            <span className="block truncate font-medium text-zinc-100">{row.display_name || row.username}</span>
            <span className="block truncate text-xs text-zinc-500">
              @{row.username}
              {row.country ? ` · ${row.country}` : ""}
            </span>
          </span>
          <span className="text-right text-zinc-400">{row.text}</span>
        </div>
      ))}
      {rows.length === 0 ? <div className="p-4 text-sm text-zinc-500">No ranked activity yet.</div> : null}
    </div>
  );
}

function Avatar({ row }: { row: LeaderboardEntry }) {
  if (row.avatar_url) {
    return <span className="h-9 w-9 rounded-md border border-line bg-cover bg-center" style={{ backgroundImage: `url(${row.avatar_url})` }} />;
  }
  return (
    <span className="grid h-9 w-9 place-items-center rounded-md border border-line bg-white/5 text-xs font-medium text-zinc-400">
      {(row.display_name || row.username).slice(0, 2).toUpperCase()}
    </span>
  );
}
