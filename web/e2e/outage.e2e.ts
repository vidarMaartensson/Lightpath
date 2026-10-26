import { expect, test } from "@playwright/test";

// The numbers come from the deterministic demo seed (network.Seed) and match
// what TestSeedScenarios in internal/impact describes.

const headline = (page: import("@playwright/test").Page) => page.getByRole("heading", { level: 1 });

test("shows the whole network with service when nothing is down", async ({ page }) => {
  await page.goto("/");

  await expect(headline(page)).toHaveText("All 1,526 customers have service.");
  await expect(page.getByRole("heading", { name: "Providers on the network" })).toBeVisible();
  await expect(page.getByText("Select a site on the map to take it down.")).toBeVisible();
});

test("taking a site down on the map shows who lost service", async ({ page }) => {
  await page.goto("/");
  await expect(headline(page)).toHaveText("All 1,526 customers have service.");

  await page.getByRole("button", { name: /^Centrum aggregation,/ }).click();

  await expect(headline(page)).toHaveText(
    "140 customers have lost service. 274 more stay online only because traffic found another path.",
  );
  await expect(page.getByRole("heading", { name: "Providers to notify" })).toBeVisible();
  await expect(page.locator(".provider").filter({ hasText: "Älvnät" })).toContainText("64 of 651 down");
  await expect(page.locator(".down-list")).toContainText("Centrum aggregation");
  await expect(page).toHaveURL(/\?down=agg-cen$/);
});

test("a prepared scenario where backup paths absorb the failure", async ({ page }) => {
  await page.goto("/");

  const scenario = page.getByRole("button", { name: "Core A fails" });
  await scenario.click();

  await expect(scenario).toHaveAttribute("aria-pressed", "true");
  await expect(headline(page)).toHaveText(
    "Nobody has lost service. 1,526 customers are running on backup paths.",
  );
});

test("a shared link opens the same outage", async ({ page }) => {
  await page.goto("/?down=agg-cen,acc-ali");

  await expect(headline(page)).toHaveText(
    "310 customers have lost service. 104 more stay online only because traffic found another path.",
  );
  const downList = page.locator(".down-list");
  await expect(downList).toContainText("Centrum aggregation");
  await expect(downList).toContainText("Ålidhem access site");
});

test("restore all sites brings everyone back", async ({ page }) => {
  await page.goto("/?down=agg-cen,acc-ali");
  await expect(headline(page)).toContainText("310 customers have lost service.");

  await page.getByRole("button", { name: "Restore all sites" }).click();

  await expect(headline(page)).toHaveText("All 1,526 customers have service.");
  await expect(page).not.toHaveURL(/down=/);
});

test("an unknown site in the link says what went wrong", async ({ page }) => {
  await page.goto("/?down=no-such-site");

  await expect(page.getByRole("status")).toContainText("Couldn't update the analysis.");
});
