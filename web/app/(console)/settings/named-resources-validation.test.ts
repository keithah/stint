import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source =
  readFileSync("components/settings/api-keys-card.tsx", "utf8") +
  readFileSync("components/settings/oauth-apps-card.tsx", "utf8") +
  readFileSync("components/settings/share-tokens-card.tsx", "utf8") +
  readFileSync("components/settings/shared.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /const canCreateAPIKey = name\.trim\(\)\.length > 0;/);
assert.match(source, /const oauthRedirectURIs = oauthRedirect\.split\("\\n"\)\.map\(\(value\) => value\.trim\(\)\)\.filter\(Boolean\);/);
assert.match(source, /const canCreateOAuthApp = oauthName\.trim\(\)\.length > 0 && oauthRedirectURIs\.length > 0 && oauthRedirectURIs\.every\(isHTTPURL\);/);
assert.match(source, /const canCreateShareToken = shareName\.trim\(\)\.length > 0;/);
assert.match(source, /create\.mutate\(name\.trim\(\)\)/);
assert.match(source, /name: oauthName\.trim\(\)/);
assert.match(source, /redirect_uris: oauthRedirectURIs/);
assert.match(source, /createShareToken\(shareName\.trim\(\)\)/);
assert.match(source, /disabled=\{create\.isPending \|\| !canCreateAPIKey\}/);
assert.match(source, /disabled=\{createApp\.isPending \|\| !canCreateOAuthApp\}/);
assert.match(source, /disabled=\{createShare\.isPending \|\| !canCreateShareToken\}/);
assert.match(source, /function isHTTPURL\(value: string\) \{\s+try \{\s+const parsed = new URL\(value\);\s+return parsed\.protocol === "http:" \|\| parsed\.protocol === "https:";\s+\} catch \{\s+return false;\s+\}\s+\}/);
assert.match(packageJSON, /app\/\(console\)\/settings\/named-resources-validation\.test\.ts/);
