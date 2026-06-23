import { existsSync, readFileSync } from "node:fs";
import assert from "node:assert/strict";

const pagePath = "app/users/[user]/page.tsx";
const layoutsPath = "components/profile-layouts.tsx";
const apiSource = readFileSync("lib/api.ts", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

if (!existsSync(pagePath)) {
  throw new Error("expected public profile route at app/users/[user]/page.tsx");
}
if (!existsSync(layoutsPath)) {
  throw new Error("expected profile layouts at components/profile-layouts.tsx");
}

const source = readFileSync(pagePath, "utf8");
const layouts = readFileSync(layoutsPath, "utf8");

assert.match(apiSource, /publicUserProfile/);
assert.match(apiSource, /publicUserStats/);
assert.match(apiSource, /publicUserSummaries/);
// The public profile exposes personal-info fields and the owner-selected layout.
assert.match(apiSource, /layout\?: ProfileLayout/);
assert.match(apiSource, /available_for_hire\?: boolean/);

assert.match(source, /useParams<\{ user: string \}>/);
assert.match(source, /publicUserProfile\(username\)/);
assert.match(source, /publicUserStats\(username, range\)/);
assert.match(source, /<ProfileView/);
assert.match(source, /Public profile unavailable/);

// The owner-selected layout drives which of the three views renders.
assert.match(layouts, /case "spotlight"/);
assert.match(layouts, /case "rail"/);
assert.match(layouts, /TerminalLayout/);
assert.match(layouts, /<SliceDonut title="Languages" rows=\{stats\?\.languages \?\? \[\]\} colors=\{languageColors\} \/>/);

assert.match(packageJSON, /lib\/public-user-api\.test\.ts/);
assert.match(packageJSON, /app\/users\/public-profile-route\.test\.ts/);
