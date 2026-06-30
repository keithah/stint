import { expect, test } from "@playwright/test";

const recipes = [
  { name: "Stint CLI", expected: "curl -fsSL https://stint.fyi/install.sh | sh" },
  { name: "WakaTime CLI", expected: "wakatime-cli --entity" },
  { name: "Codex", expected: "stint --sync-ai-activity --ai-agent codex" },
  { name: "VS Code", expected: "Install from the VS Code Marketplace" },
  { name: "JetBrains", expected: "Install from JetBrains Marketplace" },
  { name: "Vim/Neovim", expected: "Install vim-wakatime" },
  { name: "Shell CLI", expected: "curl -X POST" }
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
    await expect(page.locator("#integration-instructions")).toContainText("Install with one command");
    await expect(page.locator("#integration-instructions")).toContainText("Manual setup");
    await expect(page.locator("#integration-instructions img")).toHaveCount(1);
  }

  await expect(page.getByText("Stint client roadmap")).toHaveCount(0);
});
