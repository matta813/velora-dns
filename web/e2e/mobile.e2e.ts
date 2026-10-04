import { expect, test } from "@playwright/test";
import { signIn } from "./helpers";

test("navigates with the mobile menu and keeps sign-out reachable", async ({ page }) => {
  await signIn(page);
  await expect(page.getByRole("button", { name: /Sign out/ })).toBeVisible();
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page.getByRole("link", { name: "Query log" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Query log" })).toBeVisible();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(1);
});
