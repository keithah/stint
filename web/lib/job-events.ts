"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { me } from "@/lib/api";

const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

function currentUserEventsURL() {
  const path = "/api/v1/users/current/events";
  if (!apiBase) {
    return path;
  }
  return new URL(path, `${apiBase.replace(/\/$/, "")}/`).toString();
}

export function useJobEvents() {
  const client = useQueryClient();
  const user = useQuery({ queryKey: ["me"], queryFn: me, staleTime: 60000 });

  useEffect(() => {
    if (!user.data || typeof window === "undefined" || typeof EventSource === "undefined") {
      return;
    }

    const source = new EventSource(currentUserEventsURL(), { withCredentials: true });
    const invalidateDataDumps = () => {
      client.invalidateQueries({ queryKey: ["data-dumps"] });
      client.invalidateQueries({ queryKey: ["settings-data-dumps"] });
    };
    const invalidateCustomRulesProgress = () => {
      client.invalidateQueries({ queryKey: ["custom-rules-progress"] });
      client.invalidateQueries({ queryKey: ["custom-rules"] });
    };

    source.addEventListener("data_dumps", invalidateDataDumps);
    source.addEventListener("custom_rules_progress", invalidateCustomRulesProgress);

    return () => {
      source.removeEventListener("data_dumps", invalidateDataDumps);
      source.removeEventListener("custom_rules_progress", invalidateCustomRulesProgress);
      source.close();
    };
  }, [client, user.data]);
}
