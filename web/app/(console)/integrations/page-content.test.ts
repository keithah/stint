import { readFileSync } from "node:fs";

const source = [
  readFileSync("app/(console)/integrations/page.tsx", "utf8"),
  readFileSync("app/(console)/integrations/recipes.ts", "utf8")
].join("\n");

assertIncludes("integrations page exposes native Stint CLI install", source, "curl -fsSL https://stint.fyi/install.sh | sh");
assertIncludes("integrations page exposes one-command configured install", source, "STINT_API_URL");
assertIncludes("integrations page injects the generated key into setup", source, "STINT_API_KEY");
assertIncludes("integrations page copies generated setup after key creation", source, "copyGeneratedSetup");
assertIncludes("integrations page verifies connection with one button", source, "Verify connection");
assertIncludes("integrations page refreshes user agents during validation", source, "validateConnection");
assertIncludes("integrations page validates generated key last use", source, "latestKeyId");
assertIncludes("integrations page checks API key last_used_at", source, "last_used_at");
assertIncludes("integrations page uses Stint key placeholders", source, "stint_your_stint_key");
assertIncludes("integrations page asks where the user codes", source, "Choose where you code");
assertIncludes("integrations page has terminal category", source, "Terminal");
assertIncludes("integrations page has AI agents category", source, "AI agents");
assertIncludes("integrations page has editors category", source, "Editors");
assertIncludes("integrations page tracks active setup category", source, "activeToolCategory");
assertIncludes("integrations page has guided terminal action", source, "Install Stint");
assertIncludes("integrations page has guided agent action", source, "Install agent plugin");
assertIncludes("integrations page has guided editor action", source, "Install editor plugin");
assertIncludes("integrations page models connection status", source, "connectionStatus");
assertIncludes("integrations page shows not connected state", source, "Not connected yet");
assertIncludes("integrations page shows pending check-in state", source, "Waiting for first check-in");
assertIncludes("integrations page shows connected state", source, "Stint is connected");
assertIncludes("integrations page makes integration cards selectable", source, "setSelectedIntegration");
assertIncludes("integrations page updates the hash for selected setup cards", source, 'window.history.replaceState(null, "", `#${recipeId}`)');
assertIncludes("integrations page exposes selected instructions region", source, "integration-instructions");
assertIncludes("integrations page shows selected setup inline", source, "SetupDisclosure");
assertIncludes("integrations page offers curl install option", source, "Install with one command");
assertIncludes("integrations page offers marketplace option", source, "Install Stint marketplace plugin");
assertIncludes("integrations page offers manual fallback option", source, "Manual setup");
assertIncludes("integrations page includes screenshot assets", source, "/integrations/screenshots/");
assertIncludes("integrations page uses nontechnical guide steps", source, "Step 1");
assertIncludes("integrations page documents WakaTime CLI recipe", source, "wakatime-cli --entity");
assertIncludes("integrations page documents WakaTime CLI offline sync", source, "wakatime-cli --sync-offline-activity");
assertIncludes("integrations page documents Codex recipe", source, "stint --sync-ai-activity --ai-agent codex");
assertIncludes("integrations page documents Codex marketplace add", source, "codex plugin marketplace add https://github.com/keithah/stint.git");
assertIncludes("integrations page documents Codex marketplace install", source, "codex plugin add codex-cli-stint@stint");
assertIncludes("integrations page documents Codex heartbeat recipe", source, "--ai-agent codex");
assertIncludes("integrations page documents Codex CLI setup", source, "Codex CLI");
assertIncludes("integrations page documents Codex Desktop setup", source, "Codex Desktop");
assertIncludes("integrations page documents Claude Code", source, "Claude Code");
assertIncludes("integrations page documents Claude marketplace add", source, "claude plugin marketplace add https://github.com/keithah/stint.git");
assertIncludes("integrations page documents Claude marketplace install", source, "claude plugin i claude-code-stint@stint");
assertIncludes("integrations page documents Claude CLI setup", source, "Claude Code CLI");
assertIncludes("integrations page documents Claude Desktop setup", source, "Claude Desktop");
assertIncludes("integrations page leads with Stint marketplace plugins", source, "Choose Stint marketplace plugin");
assertIncludes("integrations page offers Stint CLI second", source, "Install Stint CLI");
assertIncludes("integrations page includes WakaTime compatibility option", source, "Use WakaTime-compatible plugin");
assertIncludes("integrations page documents Stint VS Code package", source, "Stint for VS Code");
assertIncludes("integrations page documents Stint JetBrains package", source, "Stint for JetBrains");
assertIncludes("integrations page documents model-aware token fields", source, "ai_input_tokens");
assertIncludes("integrations page documents VS Code marketplace", source, "https://marketplace.visualstudio.com/items?itemName=WakaTime.vscode-wakatime");
assertIncludes("integrations page documents JetBrains marketplace", source, "https://plugins.jetbrains.com/plugin/7425-wakatime");
assertIncludes("integrations page documents vim plugin", source, "https://github.com/wakatime/vim-wakatime");
assertIncludes("integrations page keeps advanced CLI discoverable", source, "stint data-dumps download DUMP_ID");
assertIncludes("integrations page keeps compatibility note short", source, "Stint accepts WakaTime-style API keys");
assertExcludes("integrations page no longer shows multi-step CLI validation block", source, 'stint heartbeat --entity "$PWD/README.md" --write --project my-project');
assertExcludes("integrations page removes connection health dashboard", source, "Connection health");
assertExcludes("integrations page removes extended telemetry panel", source, "Extended AI telemetry");
assertExcludes("integrations page removes status tiles", source, "StatusTile");
assertExcludes("integrations page removes screenshot previews", source, "<img");
assertExcludes("integrations page does not lead with catalog jargon", source, "Integration catalog");
assertExcludes("integrations page removes right-side detail panel", source, "DetailPanel");
assertExcludes("integrations page does not use copy setup as primary source text", source, "Copy setup, run it once");

assertExcludes("integrations page does not ask users to build Stint CLI", source, "make stint");
assertExcludes("integrations page does not expose bin-prefixed setup commands", source, "bin/stint");
assertExcludes("Codex guide must not push users to VS Code", source, "Use Stint for VS Code or Stint for JetBrains when Codex runs inside your editor");
assertExcludes("Claude guide must not push users to VS Code", source, "Use Stint for VS Code or Stint for JetBrains when Claude runs inside your editor");
assertExcludes("integrations page removes completed roadmap panel", source, "Stint client roadmap");
assertExcludes("integrations page removes roadmap data model", source, "const roadmap");
assertExcludes("integrations page removes hidden model-aware roadmap recipe", source, "model-aware-ingestion-config");
assertExcludes("integrations page removes hidden catalog roadmap recipe", source, "integration-catalog-config");

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
