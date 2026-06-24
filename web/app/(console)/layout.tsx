import type { ReactNode } from "react";
import { AppShell } from "@/components/app-shell";

// Route-group layout: wraps every console page in the query Providers + nav
// Shell exactly once, so pages no longer self-wrap with <AppShell>. The
// (console) group leaves URLs unchanged — parentheses are not path segments —
// and a new console page now gets the shell automatically by living here.
export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
