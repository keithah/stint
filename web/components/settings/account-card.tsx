"use client";

import { useMutation } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useState } from "react";
import { deleteCurrentUser } from "@/lib/api";

export function AccountCard() {
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const deleteAccount = useMutation({
    mutationFn: () => deleteCurrentUser(deleteConfirmation),
    onSuccess: () => {
      window.location.href = "/login";
    }
  });

  return (
    <section className="mt-5 rounded border border-red-900/70 bg-red-950/10 p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <h2 className="font-medium text-red-200">Danger zone</h2>
          <p className="mt-1 text-sm text-red-200/70">Delete your account, sessions, API keys, heartbeats, settings, shares, and generated data.</p>
        </div>
        <button
          className="inline-flex items-center justify-center gap-2 rounded bg-red-500 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
          onClick={() => deleteAccount.mutate()}
          disabled={deleteConfirmation !== "DELETE" || deleteAccount.isPending}
        >
          <Trash2 size={16} /> Delete account
        </button>
      </div>
      <input
        className="mt-4 w-full rounded border border-red-900/70 bg-ink px-3 py-2 text-sm outline-none focus:border-red-400"
        placeholder="DELETE"
        value={deleteConfirmation}
        onChange={(event) => setDeleteConfirmation(event.target.value)}
      />
      {deleteAccount.error ? <p className="mt-3 text-sm text-red-300">{deleteAccount.error.message}</p> : null}
    </section>
  );
}
