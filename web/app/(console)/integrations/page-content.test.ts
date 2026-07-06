import { readFileSync } from "node:fs";

const source = [
  readFileSync("app/(console)/integrations/page.tsx", "utf8"),
  readFileSync("app/(console)/integrations/recipes.ts", "utf8")
].join("\n");

assertIncludes("integrations page exposes native Stint CLI install", source, "curl -fsSL https://stint.fyi/install.sh | sh");
assertIncludes("integrations page exposes one-command configured install", source, "STINT_API_URL");
assertIncludes("integrations page injects the generated key into setup", source, "STINT_API_KEY");
assertIncludes("integrations page uses Stint key placeholders", source, "stint_your_stint_key");
assertIncludes("integrations page documents native Stint config", source, "~/.stint.cfg");
assertIncludes("integrations page shows Stint CLI connected state", source, "Yes, Stint CLI is connected");
assertIncludes("integrations page makes integration cards selectable", source, "setSelectedIntegration");
assertIncludes("integrations page links every setup card to a stable hash", source, 'const href = `#${recipeId}`');
assertIncludes("integrations page exposes selected instructions region", source, "integration-instructions");
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
