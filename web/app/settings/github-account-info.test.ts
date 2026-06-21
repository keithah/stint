import { readFileSync } from "node:fs";

const source = readFileSync("app/settings/page.tsx", "utf8");

assertIncludes("settings labels GitHub account panel", source, "GitHub account");
assertIncludes("settings renders GitHub username", source, "github_username");
assertIncludes("settings renders GitHub full name", source, "full_name");
assertIncludes("settings renders GitHub email", source, "email");
assertIncludes("settings uses avatar URL when present", source, "avatar_url");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
