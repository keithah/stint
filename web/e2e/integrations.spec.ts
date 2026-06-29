import { expect, test } from "@playwright/test";

const recipes = [
  { name: "Stint CLI", expected: "bin/stint config init" },
  { name: "WakaTime CLI", expected: "wakatime-cli --entity" },
  { name: "Codex", expected: "bin/stint --sync-ai-activity --agent codex" },
  { name: "VS Code", expected: "Install the WakaTime extension from the VS Code Marketplace." },
  { name: "JetBrains", expected: "Install the WakaTime plugin from JetBrains Marketplace." },
  { name: "Vim/Neovim", expected: "Install vim-wakatime for Vim or Neovim." },
  { name: "Shell CLI", expected: "curl -X POST" }
];

const roadmapRecipes = [
  { name: "Model-aware ingestion", expected: "ai_input_tokens" },
  { name: "Native Stint CLI", expected: "bin/stint config init" },
  { name: "Integration catalog", expected: "Create integration key" }
];

test("integration names reveal full setup instructions", async ({ page }) => {
  await page.route("**/api/v1/meta", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { api_url: "http://127.0.0.1:3310/api/v1", base_url: "http://127.0.0.1:3310", hostname: "playwright", ip: "127.0.0.1", version: "test" } })
    });
  });
  await page.route("**/api/v1/api_keys", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [] }) });
  });
  await page.route("**/api/v1/editors", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [] }) });
  });
  await page.route("**/api/v1/users/current/user_agents", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [] }) });
  });
  await page.route("**/api/v1/auth/me", async (route) => {
    await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "unauthorized" }) });
  });

  await page.goto("/integrations");

  const stintCard = page.getByRole("button", { name: "Show Stint CLI integration instructions" });
  await expect(stintCard).toContainText("live");

  for (const recipe of recipes) {
    await page.getByRole("button", { name: `Show ${recipe.name} integration instructions` }).click();
    await expect(page.locator("#integration-instructions")).toContainText(recipe.expected);
  }

  for (const recipe of roadmapRecipes) {
    await page.getByRole("button", { name: `Show ${recipe.name} roadmap instructions` }).click();
    await expect(page.locator("#integration-instructions")).toContainText(recipe.expected);
  }
});
