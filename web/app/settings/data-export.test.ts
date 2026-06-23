import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const source =
  readFileSync(join(process.cwd(), "components/settings/data-export-card.tsx"), "utf8") +
  readFileSync(join(process.cwd(), "components/settings/wakatime-import-card.tsx"), "utf8");
const packageJSON = readFileSync(join(process.cwd(), "package.json"), "utf8");

assert.match(source, /createDataDump/);
assert.match(source, /listDataDumps/);
assert.match(source, /dataDumpDownloadURL/);
assert.match(source, /dataDumpExpiryText/);
assert.match(source, /dataDumpIsDownloadable/);
assert.match(source, /hasPendingDumps/);
assert.match(source, /const \[settingsDumpType, setSettingsDumpType\]/);
assert.match(source, /queryKey: \["settings-data-dumps"\]/);
assert.match(source, /createSettingsDump\.mutate\(\)/);
assert.match(source, />Data export</);
assert.match(source, /Generate export/);
assert.match(source, /href=\{isReady \? dataDumpDownloadURL\(dump\.download_url\) : "#"\}/);
assert.match(source, /const isReady = dataDumpIsDownloadable\(dump\)/);
assert.match(source, /const expiryText = dataDumpExpiryText\(dump\)/);
assert.match(source, /json\.gz/);
assert.match(packageJSON, /app\/settings\/data-export\.test\.ts/);
