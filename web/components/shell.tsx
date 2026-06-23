"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, BarChart3, Boxes, Coins, FileDown, Goal, KeyRound, LayoutDashboard, LogIn, PlugZap, Trophy } from "lucide-react";
import { me } from "@/lib/api";

export function Shell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const user = useQuery({ queryKey: ["me"], queryFn: me, retry: false, staleTime: 60000 });
  const isLoggedIn = user.isSuccess;
  const accountHref = isLoggedIn ? "/settings" : "/login";
  const AccountIcon = isLoggedIn ? KeyRound : LogIn;
  const accountLabel = isLoggedIn ? "Settings" : "Login";
  return (
    <main className="min-h-screen">
      <header className="sticky top-0 z-40 border-b border-line bg-rail/95 px-4 py-3 backdrop-blur md:hidden">
        <div className="mb-3 flex items-center justify-between gap-3">
          <Link className="flex min-w-0 items-center gap-3" href="/" aria-label="Stint home">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-accent/35 bg-accent/15 text-accent">
              <Activity size={20} strokeWidth={2.5} />
            </div>
            <div className="min-w-0">
              <div className="truncate text-base font-semibold tracking-wide">Stint</div>
              <div className="truncate text-[11px] uppercase text-zinc-500">Operations console</div>
            </div>
          </Link>
          <Link
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded border border-line text-zinc-300 hover:bg-white/5"
            href={accountHref}
            aria-label={accountLabel}
          >
            <AccountIcon size={17} />
          </Link>
        </div>
        <nav className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 text-sm" aria-label="Mobile navigation">
          {navItems.map((item) => (
            <Link key={item.href} className={mobileNavClass(isActive(pathname, item.href))} href={item.href}>
              <item.icon size={16} /> {item.label}
            </Link>
          ))}
        </nav>
      </header>
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-line bg-rail/95 px-5 py-6 backdrop-blur md:block">
        <Link className="mb-10 flex items-center gap-3" href="/" aria-label="Stint home">
          <div className="flex h-10 w-10 items-center justify-center rounded-md border border-accent/35 bg-accent/15 text-accent shadow-glow">
            <Activity size={22} strokeWidth={2.5} />
          </div>
          <div>
            <div className="text-lg font-semibold tracking-wide">Stint</div>
            <div className="text-xs uppercase text-zinc-500">Operations console</div>
          </div>
        </Link>
        <nav className="space-y-2 text-sm">
          {navItems.map((item) => (
            <Link key={item.href} className={desktopNavClass(isActive(pathname, item.href))} href={item.href}>
              <item.icon size={17} /> {item.label}
            </Link>
          ))}
          <Link className="mt-5 flex items-center gap-3 rounded-md border border-line px-3 py-2 text-zinc-300 hover:border-accent/50 hover:bg-white/5" href={accountHref}>
            <AccountIcon size={17} /> {accountLabel}
          </Link>
        </nav>
      </aside>
      <section className="md:pl-64">{children}</section>
    </main>
  );
}

const navItems = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/projects", label: "Projects", icon: Boxes },
  { href: "/integrations", label: "Integrations", icon: PlugZap },
  { href: "/insights", label: "Insights", icon: BarChart3 },
  { href: "/ai-costs", label: "AI Costs", icon: Coins },
  { href: "/goals", label: "Goals", icon: Goal },
  { href: "/leaderboards", label: "Leaderboards", icon: Trophy },
  { href: "/reports", label: "Reports", icon: FileDown },
  { href: "/settings", label: "Settings", icon: KeyRound }
] as const;

function isActive(pathname: string, href: string) {
  return pathname === href || pathname.startsWith(`${href}/`);
}

function desktopNavClass(active: boolean) {
  return `flex items-center gap-3 rounded-md px-3 py-2 transition ${active ? "border border-accent/30 bg-accent/10 text-accent" : "text-zinc-200 hover:bg-white/5"}`;
}

function mobileNavClass(active: boolean) {
  return `flex shrink-0 items-center gap-2 rounded-md border px-3 py-2 transition ${active ? "border-accent/40 bg-accent/15 text-accent" : "border-line bg-panel text-zinc-100 hover:bg-white/5"}`;
}
