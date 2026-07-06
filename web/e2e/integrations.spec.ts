import { expect, test } from "@playwright/test";

const recipes = [
  { name: "Stint CLI", expected: "curl -fsSL https://stint.fyi/install.sh | STINT_API_URL", stintOwned: false, compatibility: true },
  { name: "Codex", expected: "codex plugin add codex-cli-stint@stint", stintOwned: true, compatibility: true },
  { name: "Claude Code", expected: "claude plugin i claude-code-stint@stint", stintOwned: true, compatibility: true },
  { name: "VS Code", expected: "Stint for VS Code", stintOwned: true, compatibility: true },
  { name: "JetBrains", expected: "Stint for JetBrains", stintOwned: true, compatibility: true },
  { name: "Vim/Neovim", expected: "Install vim-wakatime", stintOwned: false, compatibility: false }
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

  await expect(page.getByRole("heading", { name: "Connect Stint" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Choose where you code" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Terminal/ })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "Install Stint" })).toBeVisible();
  await expect(page.getByText("Not connected yet")).toBeVisible();

  const stintCard = page.getByRole("button", { name: "Show Stint CLI integration instructions" });
  await expect(stintCard).toContainText("Open");
  await expect(page.locator("#integration-instructions")).not.toContainText("curl -fsSL");
  await page.getByText("Show command").click();
  await expect(page.locator("#integration-instructions")).toContainText("curl -fsSL");

  for (const recipe of recipes) {
    if (recipe.name === "Codex" || recipe.name === "Claude Code") {
      await page.getByRole("button", { name: /AI agents/ }).click();
      await expect(page.getByRole("button", { name: /AI agents/ })).toHaveAttribute("aria-pressed", "true");
      await expect(page.getByRole("button", { name: "Install agent plugin" })).toBeVisible();
    } else if (recipe.name === "VS Code" || recipe.name === "JetBrains" || recipe.name === "Vim/Neovim") {
      await page.getByRole("button", { name: /Editors/ }).click();
      await expect(page.getByRole("button", { name: /Editors/ })).toHaveAttribute("aria-pressed", "true");
      await expect(page.getByRole("button", { name: "Install editor plugin" })).toBeVisible();
    } else {
      await page.getByRole("button", { name: /Terminal/ }).click();
      await expect(page.getByRole("button", { name: /Terminal/ })).toHaveAttribute("aria-pressed", "true");
      await expect(page.getByRole("button", { name: "Install Stint" })).toBeVisible();
    }
    await page.getByRole("button", { name: `Show ${recipe.name} integration instructions` }).click();
    const setupToggle = page.locator("#integration-instructions button[aria-expanded]").first();
    if ((await setupToggle.getAttribute("aria-expanded")) !== "true") {
      await setupToggle.click();
    }
    await expect(page.locator("#integration-instructions")).toContainText(recipe.expected);
    if (recipe.name === "Codex" || recipe.name === "Claude Code") {
      await expect(page.locator("#integration-instructions")).toContainText("Choose Stint marketplace plugin");
      await expect(page.locator("#integration-instructions")).toContainText("Install Stint CLI");
      await expect(page.locator("#integration-instructions")).not.toContainText("Install Stint for VS Code");
    }
    if (recipe.stintOwned) {
      await expect(page.locator("#integration-instructions")).toContainText("Install Stint marketplace plugin");
    }
    if (recipe.compatibility) {
      await expect(page.locator("#integration-instructions")).toContainText("Use WakaTime-compatible plugin");
    }
    await expect(page.locator("#integration-instructions img")).toHaveCount(0);
  }

  await expect(page.getByRole("button", { name: "Show Shell CLI integration instructions" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Show WakaTime CLI integration instructions" })).toHaveCount(0);
  await expect(page.getByText("Stint client roadmap")).toHaveCount(0);
  const verifyButton = page.getByRole("button", { name: "Verify connection" });
  await expect(verifyButton).toBeVisible();
  const verifyButtonBox = await verifyButton.boundingBox();
  expect(verifyButtonBox).not.toBeNull();
  expect(verifyButtonBox!.x + verifyButtonBox!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
});
