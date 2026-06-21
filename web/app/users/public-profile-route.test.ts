import { existsSync, readFileSync } from "node:fs";
import assert from "node:assert/strict";

const pagePath = "app/users/[user]/page.tsx";
const apiSource = readFileSync("lib/api.ts", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

if (!existsSync(pagePath)) {
  throw new Error("expected public profile route at app/users/[user]/page.tsx");
}

const source = readFileSync(pagePath, "utf8");

assert.match(apiSource, /publicUserProfile/);
assert.match(apiSource, /publicUserStats/);
assert.match(apiSource, /publicUserSummaries/);
assert.match(source, /useParams<\{ user: string \}>/);
assert.match(source, /publicUserProfile\(username\)/);
assert.match(source, /publicUserStats\(username, range\)/);
assert.match(source, /publicUserSummaries\(username, startDate, endDate\)/);
assert.match(source, /Public profile unavailable/);
assert.match(source, /<SliceDonut title="Languages" rows=\{data\?\.languages \?\? \[\]\} colors=\{languageColors\} \/>/);
assert.match(packageJSON, /lib\/public-user-api\.test\.ts/);
assert.match(packageJSON, /app\/users\/public-profile-route\.test\.ts/);
