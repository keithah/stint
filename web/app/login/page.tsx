"use client";

import { useMutation } from "@tanstack/react-query";
import { Github, KeyRound } from "lucide-react";
import { seedDevKey } from "@/lib/api";
import { Providers } from "@/components/providers";

export default function LoginPage() {
  return (
    <Providers>
      <LoginContent />
    </Providers>
  );
}

function LoginContent() {
  const seed = useMutation({
    mutationFn: seedDevKey,
    onSuccess: () => {
      window.location.href = "/settings";
    }
  });

  return (
    <main className="grid min-h-screen place-items-center px-6">
      <section className="w-full max-w-md rounded border border-line bg-panel p-6 shadow-glow">
        <div className="mb-6">
          <div className="mb-3 inline-flex h-11 w-11 items-center justify-center rounded bg-accent text-ink">
            <KeyRound size={22} />
          </div>
          <h1 className="text-2xl font-semibold">Sign in to Stint</h1>
          <p className="mt-2 text-sm leading-6 text-zinc-400">Use GitHub for normal sessions or seed a local key for plugin smoke tests.</p>
        </div>
        <div className="space-y-3">
          <a className="flex w-full items-center justify-center gap-2 rounded bg-zinc-100 px-4 py-3 font-medium text-ink hover:bg-white" href="/auth/github/login">
            <Github size={18} /> Continue with GitHub
          </a>
          <button
            className="flex w-full items-center justify-center gap-2 rounded border border-line px-4 py-3 font-medium text-zinc-100 hover:bg-white/5 disabled:opacity-60"
            onClick={() => seed.mutate()}
            disabled={seed.isPending}
          >
            <KeyRound size={18} /> Create local dev key
          </button>
        </div>
        {seed.error ? <p className="mt-4 text-sm text-red-300">{seed.error.message}</p> : null}
      </section>
    </main>
  );
}
