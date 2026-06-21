import { existsSync, readFileSync } from "node:fs";

const pagePath = "app/share/[userOrToken]/page.tsx";
const userScopedPagePath = "app/share/[userOrToken]/[token]/page.tsx";
const apiSource = readFileSync("lib/api.ts", "utf8");

if (!existsSync(pagePath)) {
  throw new Error("expected token-only share route at app/share/[userOrToken]/page.tsx");
}

if (!existsSync(userScopedPagePath)) {
  throw new Error("expected user-scoped share route to reuse app/share/[userOrToken]/[token]/page.tsx");
}

const pageSource = readFileSync(pagePath, "utf8");

assertIncludes("token route calls token-only helper", pageSource, "publicShareStatsByToken");
assertIncludes("token route maps single share segment to token", pageSource, "useParams<{ userOrToken: string }>");
assertIncludes("token route reads neutral first param", pageSource, "const token = params.userOrToken");
assertIncludes("API client exposes token-only share helper", apiSource, "publicShareStatsByToken");
assertIncludes("API client calls token-only share endpoint", apiSource, "/api/v1/share/");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
