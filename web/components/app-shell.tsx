"use client";
import type { ReactNode } from "react";
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";

// Wraps every console page in the query Providers + nav Shell. The idiomatic
// App Router approach is an `app/(console)/layout.tsx` route group, but that
// requires moving the console pages into the group, which breaks ~43 tests that
// read page files by hardcoded path. AppShell gets the same de-duplication with
// zero file moves. Follow-up: introduce the route-group layout once those tests
// no longer pin source paths.
export function AppShell({ children }: { children: ReactNode }) {
  return <Providers><Shell>{children}</Shell></Providers>;
}
