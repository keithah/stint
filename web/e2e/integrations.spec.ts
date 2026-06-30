import { expect, test } from "@playwright/test";

const recipes = [
  { name: "Stint CLI", expected: "curl -fsSL https://stint.fyi/install.sh | sh", stintOwned: false, compatibility: true },
  { name: "WakaTime CLI", expected: "wakatime-cli --entity", stintOwned: false, compatibility: true },
  { name: "Codex", expected: "Codex Desktop", stintOwned: true, compatibility: true },
  { name: "Claude Code", expected: "Claude Desktop", stintOwned: true, compatibility: true },
  { name: "VS Code", expected: "Stint for VS Code", stintOwned: true, compatibility: true },
  { name: "JetBrains", expected: "Stint for JetBrains", stintOwned: true, compatibility: true },
  { name: "Vim/Neovim", expected: "Install vim-wakatime", stintOwned: false, compatibility: false },
  { name: "Shell CLI", expected: "curl -X POST", stintOwned: false, compatibility: false }
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
    if (recipe.stintOwned) {
      await expect(page.locator("#integration-instructions")).toContainText("Install Stint-owned plugin");
    }
    if (recipe.compatibility) {
      await expect(page.locator("#integration-instructions")).toContainText("Use WakaTime-compatible plugin");
    }
    await expect(page.locator("#integration-instructions img")).toHaveCount(1);
  }

  await expect(page.getByText("Stint client roadmap")).toHaveCount(0);
});
