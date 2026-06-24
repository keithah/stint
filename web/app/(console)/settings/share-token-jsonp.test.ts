import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source =
  readFileSync("components/settings/share-tokens-card.tsx", "utf8") +
  readFileSync("components/settings/shared.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /shareStatsJSONPURL/);
assert.match(source, /StintEmbed\.render/);
assert.match(source, /\/share\/\$\{encodeURIComponent\(token\)\}\/stats/);
assert.match(source, /callback/);
assert.match(source, /JSONP stats endpoint/);
assert.match(packageJSON, /app\/\(console\)\/settings\/share-token-jsonp\.test\.ts/);
