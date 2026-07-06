import { readFileSync } from "node:fs";

const aiCompat = readFileSync("../docs/wakatime-ai-compat.md", "utf8");
const pluginSetup = readFileSync("../docs/PLUGIN_SETUP.md", "utf8");
const readme = readFileSync("../README.md", "utf8");

assertIncludes("AI compatibility doc marks stats read aliases implemented", aiCompat, "Stats response (`GET .../stats`) AI block | Implemented");
assertIncludes("AI compatibility doc marks durations AI fields implemented", aiCompat, "Durations response (`GET .../durations`) AI fields | Implemented");
assertIncludes("AI compatibility doc marks prompt insights implemented", aiCompat, "Prompt Insights (per-session avg/median) | Implemented");
assertExcludes("AI compatibility doc no longer claims stats aliases are missing", aiCompat, "Different field names; missing metrics");
assertExcludes("AI compatibility doc no longer claims durations AI fields are absent", aiCompat, "Not exposed at all");
assertExcludes("AI compatibility doc no longer claims prompt insights are missing", aiCompat, "Not computed");

assertIncludes("plugin setup documents configured release install", pluginSetup, "STINT_API_URL=");
assertIncludes("plugin setup documents configured API key install", pluginSetup, "STINT_API_KEY=");
assertIncludes("plugin setup documents setup verification", pluginSetup, "stint doctor");
assertIncludes("plugin setup documents smoke heartbeat", pluginSetup, "stint heartbeat");
assertIncludes("plugin setup documents AI sync", pluginSetup, "stint --sync-ai-activity --ai-agent codex");
assertIncludes("plugin setup documents expanded dependency detectors", pluginSetup, "Haxe, HTML, Objective-C");
assertIncludes("plugin setup documents VB.NET dependency detector", pluginSetup, "VB.NET");
assertIncludes("plugin setup documents package manager dependency markers", pluginSetup, "package-manager markers (`npm` or `bower`)");
assertIncludes("plugin setup documents data dump downloads", pluginSetup, "stint data-dumps download DUMP_ID");
assertIncludes("plugin setup documents offline sync", pluginSetup, "stint offline sync");
assertIncludes("plugin setup documents ordered project maps", pluginSetup, "first matching entry in file order");
assertIncludes("plugin setup documents ordered api_urls fanout", pluginSetup, "every matching entry in file order");

assertIncludes("README documents configured release install", readme, "STINT_API_URL=");
assertIncludes("README documents configured API key install", readme, "STINT_API_KEY=");
assertIncludes("README documents doctor command", readme, "stint doctor");
assertIncludes("README documents data dump downloads", readme, "stint data-dumps download DUMP_ID");
assertIncludes("README documents offline sync command", readme, "stint offline sync");
assertIncludes("README documents ordered WakaTime maps", readme, "first matching entry in file order");
assertIncludes("README documents api_urls ordered fanout", readme, "all matching `api_urls` entries fan out in file order");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}

function assertExcludes(name: string, sourceText: string, needle: string) {
  if (sourceText.includes(needle)) {
    throw new Error(`${name}: expected source not to include ${needle}`);
  }
}
