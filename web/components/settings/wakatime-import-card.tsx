"use client";

import { useMutation } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useState } from "react";
import { importWakaTimeDump } from "@/lib/api";

export function WakaTimeImportCard() {
  const [importFile, setImportFile] = useState<File | null>(null);
  const importDump = useMutation({
    mutationFn: () => {
      if (!importFile) {
        throw new Error("Choose an activity JSON dump first");
      }
      return importWakaTimeDump(importFile);
    }
  });

  return (
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
  );
}
