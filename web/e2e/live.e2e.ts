import { expect, test } from "@playwright/test";

// The events service in these tests fails a site every couple of seconds, so
// the live view has something to show quickly. Which site fails is random;
// the tests check the shape of what is shown, not the numbers.

test("opens in the live view and shows events as they happen", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("button", { name: "Live", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  const feed = page.getByRole("region", { name: "Live events" });
  await expect(feed.getByRole("status")).toHaveText("Live");

  const rows = feed.locator(".feed-row");
  await expect(rows.first()).toContainText(/went down|is back/, { timeout: 15_000 });
  // New rows arrive with their numbers pending; each gets them once the
  // impact service has answered.
  await expect(
    rows.filter({ hasText: /lost service|have service again|on backup paths|No change in service/ }).first(),
  ).toBeVisible({ timeout: 15_000 });
  await expect(page).not.toHaveURL(/down=|mode=/);
});

test("clicking the live map explains how to experiment", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("button", { name: "Live", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );

  await page.getByRole("button", { name: /^Core A,/ }).click();

  const hint = page.getByText("The map follows the live network.");
  await expect(hint).toBeVisible();
  await expect(page.getByRole("button", { name: "Restore all sites" })).toHaveCount(0);

  await page.getByRole("button", { name: "Switch to sandbox" }).click();
  await expect(page.getByRole("button", { name: "Sandbox" })).toHaveAttribute("aria-pressed", "true");
  await expect(hint).toBeHidden();
  await expect(page.getByRole("button", { name: "Core A fails" })).toBeVisible();
  await expect(page).toHaveURL(/mode=sandbox/);
});

test("a shared scenario opens in the sandbox", async ({ page }) => {
  await page.goto("/?down=agg-cen");

  await expect(page.getByRole("button", { name: "Sandbox" })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("heading", { level: 1 })).toContainText("140 customers have lost service.");
});
