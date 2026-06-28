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

assertIncludes("plugin setup documents API key management", pluginSetup, 'bin/stint api-keys create "Editor key" --scope write_heartbeats --scope read_stats');
assertIncludes("plugin setup documents OAuth app management", pluginSetup, 'bin/stint oauth-apps create "Local OAuth app" --redirect-uri http://localhost:3000/callback --scope read_stats');
assertIncludes("plugin setup documents OAuth token exchange", pluginSetup, "bin/stint oauth token --client-id OAUTH_CLIENT_ID --client-secret OAUTH_CLIENT_SECRET --code AUTH_CODE --redirect-uri http://localhost:3000/callback");
assertIncludes("plugin setup documents account read", pluginSetup, "bin/stint account");
assertIncludes("plugin setup documents account delete command", pluginSetup, "bin/stint account delete --confirm");
assertIncludes("plugin setup documents public API metadata", pluginSetup, "bin/stint meta");
assertIncludes("plugin setup documents filtered leaders", pluginSetup, "bin/stint leaders --language Go --country US");
assertIncludes("plugin setup documents public user stats", pluginSetup, "bin/stint users public-username stats last_7_days");
assertIncludes("plugin setup documents public user stats range flag", pluginSetup, "bin/stint users public-username stats --range last_30_days");
assertIncludes("plugin setup documents public user summaries window", pluginSetup, "bin/stint users public-username summaries --start 2026-06-01 --end 2026-06-30");
assertIncludes("plugin setup documents share stats", pluginSetup, "bin/stint share SHARE_TOKEN stats");
assertIncludes("plugin setup documents share stats range flag", pluginSetup, "bin/stint share SHARE_TOKEN stats --range last_7_days");
assertIncludes("plugin setup documents share summaries window", pluginSetup, "bin/stint share SHARE_TOKEN summaries --start 2026-06-01 --end 2026-06-30");
assertIncludes("plugin setup documents expanded dependency detectors", pluginSetup, "Haxe, HTML, Objective-C");
assertIncludes("plugin setup documents VB.NET dependency detector", pluginSetup, "VB.NET");
assertIncludes("plugin setup documents package manager dependency markers", pluginSetup, "package-manager markers (`npm` or `bower`)");
assertIncludes("plugin setup documents operational health", pluginSetup, "bin/stint health ingestion");
assertIncludes("plugin setup documents local dev jobs", pluginSetup, "bin/stint dev leaderboard-update --range last_7_days");
assertIncludes("plugin setup documents bulk external duration delete", pluginSetup, "bin/stint external-durations delete --ids id-1,id-2");
assertIncludes("plugin setup documents data dump downloads", pluginSetup, "bin/stint data-dumps download DUMP_ID");
assertIncludes("plugin setup documents daily data dump creation", pluginSetup, "bin/stint data-dumps create daily");
assertIncludes("plugin setup documents WakaTime import stdin", pluginSetup, "bin/stint import wakatime --stdin");
assertIncludes("plugin setup documents offline print command", pluginSetup, "bin/stint offline print");
assertIncludes("plugin setup documents ordered project maps", pluginSetup, "first matching entry in file order");
assertIncludes("plugin setup documents ordered api_urls fanout", pluginSetup, "every matching entry in file order");

assertIncludes("README documents doctor command", readme, "bin/stint doctor");
assertIncludes("README documents collect command", readme, "bin/stint collect");
assertIncludes("README documents account delete command", readme, "bin/stint account delete --confirm");
assertIncludes("README documents bulk external duration delete", readme, "bin/stint external-durations delete --ids id-1,id-2");
assertIncludes("README documents daily data dump creation", readme, "bin/stint data-dumps create daily");
assertIncludes("README documents offline count command", readme, "bin/stint offline count");
assertIncludes("README documents offline print command", readme, "bin/stint offline print");
assertIncludes("README documents offline sync command", readme, "bin/stint offline sync");
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
