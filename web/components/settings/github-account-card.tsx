"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { LogOut } from "lucide-react";
import { logout, me } from "@/lib/api";
import { SecondaryButton } from "@/components/ui";

export function GitHubAccountCard() {
  const user = useQuery({ queryKey: ["me"], queryFn: me, });
  const publicHandle = (user.data?.data.public_username?.trim() || user.data?.data.github_username || "username").replace(/^@/, "");
  const publicProfilePath = `/@${publicHandle}`;
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      window.location.href = "/login";
    }
  });

  return (
    <section className="mb-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
        <div className="flex min-w-0 items-center gap-4">
          <a
            href={publicProfilePath}
            target="_blank"
            rel="noreferrer"
            title="View your public profile"
            className="flex h-14 w-14 shrink-0 items-center justify-center rounded border border-line bg-ink bg-cover bg-center text-lg font-semibold text-zinc-300 transition hover:ring-2 hover:ring-accent"
            style={user.data?.data.avatar_url ? { backgroundImage: `url(${user.data.data.avatar_url})` } : undefined}
          >
            {user.data?.data.avatar_url ? "" : (user.data?.data.github_username ?? "?").slice(0, 1).toUpperCase()}
          </a>
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
          <SecondaryButton
            className="disabled:opacity-60"
            onClick={() => signOut.mutate()}
            disabled={signOut.isPending}
          >
            <LogOut size={16} /> Sign out
          </SecondaryButton>
        </div>
      </div>
    </section>
  );
}
