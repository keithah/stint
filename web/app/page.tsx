import Link from "next/link";
import { Activity, ArrowRight, BarChart3, Bot, Github, KeyRound, Trophy } from "lucide-react";

export default function Home() {
  return (
    <main className="min-h-screen bg-ink text-zinc-100">
      <header className="border-b border-line/80 bg-rail/80 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center justify-between gap-4 px-5 py-4 lg:px-8">
          <Link className="flex items-center gap-3" href="/" aria-label="Stint home">
            <span className="grid h-10 w-10 place-items-center rounded-md border border-accent/45 bg-accent/15 text-accent shadow-glow">
              <Activity size={22} strokeWidth={2.5} />
            </span>
            <span>
              <span className="block text-lg font-semibold tracking-wide">Stint</span>
              <span className="block text-xs uppercase tracking-[0.16em] text-zinc-500">WakaTime-compatible</span>
            </span>
          </Link>
          <nav className="hidden items-center gap-2 text-sm text-zinc-300 md:flex" aria-label="Public navigation">
            <Link className="rounded px-3 py-2 hover:bg-white/5 hover:text-white" href="/leaderboards">Leaderboards</Link>
            <Link className="rounded px-3 py-2 hover:bg-white/5 hover:text-white" href="/login">Login</Link>
            <Link className="inline-flex items-center gap-2 rounded-md bg-accent px-4 py-2 font-medium text-ink hover:bg-sky-300" href="/auth/github/login">
              <Github size={16} /> GitHub
            </Link>
          </nav>
          <Link className="inline-flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-ink md:hidden" href="/login">
            Login <ArrowRight size={15} />
          </Link>
        </div>
      </header>

      <section className="mx-auto grid max-w-7xl gap-8 px-5 py-10 lg:grid-cols-[0.78fr_1.22fr] lg:px-8 lg:py-14">
        <div className="flex flex-col justify-center">
          <div className="mb-4 inline-flex w-fit items-center gap-2 rounded-md border border-accent/25 bg-accent/10 px-3 py-1 text-xs font-medium uppercase tracking-[0.16em] text-accent">
            <Bot size={14} /> Self-hosted coding telemetry
          </div>
          <h1 className="max-w-xl text-5xl font-semibold leading-tight tracking-tight text-white sm:text-6xl">
            Stint
          </h1>
          <p className="mt-5 max-w-xl text-base leading-7 text-zinc-300">
            A private WakaTime-compatible activity console for editor heartbeats, project rankings, AI coding metrics, and model-aware cost tracking.
          </p>
          <div className="mt-7 flex flex-col gap-3 sm:flex-row">
            <Link className="inline-flex items-center justify-center gap-2 rounded-md bg-accent px-5 py-3 text-sm font-semibold text-ink hover:bg-sky-300" href="/auth/github/login">
              <Github size={18} /> Continue with GitHub
            </Link>
            <Link className="inline-flex items-center justify-center gap-2 rounded-md border border-line bg-panel px-5 py-3 text-sm font-medium text-zinc-100 hover:border-accent/50 hover:bg-white/5" href="/leaderboards">
              <Trophy size={18} /> View leaderboards
            </Link>
          </div>
          <div className="mt-8 grid gap-3 sm:grid-cols-3">
            <LandingMetric label="API" value="/api/v1" />
            <LandingMetric label="Auth" value="GitHub + API keys" />
            <LandingMetric label="AI" value="Tokens + costs" />
          </div>
        </div>

        <DashboardPreview />
      </section>
    </main>
  );
}

function LandingMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-line bg-panel/80 p-3">
      <div className="text-[11px] uppercase tracking-[0.16em] text-zinc-500">{label}</div>
      <div className="mt-2 text-sm font-medium text-zinc-100">{value}</div>
    </div>
  );
}

function DashboardPreview() {
  const days = [34, 62, 51, 78, 44, 92, 18];
  const projects = [
    { name: "stint", color: "bg-sky-400", width: "w-[92%]" },
    { name: "codex", color: "bg-emerald-400", width: "w-[76%]" },
    { name: "streamdiff", color: "bg-orange-400", width: "w-[54%]" }
  ];
  return (
    <div className="rounded-lg border border-line bg-[#0a1018] p-3 shadow-[0_28px_90px_rgba(0,0,0,0.55)]">
      <div className="mb-3 flex items-center justify-between border-b border-line pb-3">
        <div className="flex items-center gap-2">
          <span className="h-3 w-3 rounded-full bg-red-400" />
          <span className="h-3 w-3 rounded-full bg-amber-300" />
          <span className="h-3 w-3 rounded-full bg-emerald-400" />
        </div>
        <span className="rounded border border-line bg-ink px-3 py-1 text-xs text-zinc-400">Last 7 Days</span>
      </div>
      <div className="grid gap-3 md:grid-cols-[170px_1fr]">
        <aside className="hidden rounded-md bg-rail p-3 text-sm text-zinc-400 md:block">
          {["Dashboard", "Projects", "AI", "Insights", "Leaderboards"].map((item, index) => (
            <div key={item} className={`mb-1 rounded px-3 py-2 ${index === 0 ? "bg-accent/15 text-accent" : ""}`}>{item}</div>
          ))}
        </aside>
        <div className="space-y-3">
          <div className="rounded-md border border-line bg-panel p-5">
            <div className="text-xs uppercase tracking-[0.16em] text-zinc-500">Activity overview</div>
            <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
              <div className="text-4xl font-semibold text-white">53 hrs 19 mins</div>
              <div className="grid grid-cols-3 gap-2 text-xs text-zinc-400">
                <MiniTile label="Today" value="2h 7m" />
                <MiniTile label="Average" value="8h 53m" />
                <MiniTile label="Top day" value="Fri" />
              </div>
            </div>
          </div>
          <div className="grid gap-3 lg:grid-cols-[0.8fr_1.2fr]">
            <div className="rounded-md border border-line bg-panel p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium"><Bot size={16} className="text-accent" /> AI coding</div>
              <div className="grid place-items-center">
                <div className="grid h-28 w-28 place-items-center rounded-full border-[12px] border-accent text-center">
                  <span className="text-2xl font-semibold">100%</span>
                  <span className="text-xs text-zinc-500">AI-driven</span>
                </div>
              </div>
            </div>
            <div className="rounded-md border border-line bg-panel p-4">
              <div className="mb-3 flex items-center gap-2 text-sm font-medium"><BarChart3 size={16} className="text-accent" /> Projects</div>
              <div className="flex h-36 items-end gap-3 border-b border-line px-2">
                {days.map((height, index) => (
                  <span key={index} className="flex-1 rounded-t bg-accent/85" style={{ height: `${height}%` }} />
                ))}
              </div>
            </div>
          </div>
          <div className="grid gap-3 md:grid-cols-3">
            {projects.map((project) => (
              <div key={project.name} className="rounded-md border border-line bg-panel p-4">
                <div className="mb-3 text-sm font-medium text-zinc-100">{project.name}</div>
                <div className="h-1.5 rounded bg-white/5">
                  <div className={`h-1.5 rounded ${project.color} ${project.width}`} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function MiniTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-ink px-3 py-2">
      <div className="text-[10px] uppercase text-zinc-600">{label}</div>
      <div className="mt-1 font-semibold text-zinc-100">{value}</div>
    </div>
  );
}
