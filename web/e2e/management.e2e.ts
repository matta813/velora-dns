import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { expect, test } from "@playwright/test";
import { apiSignIn, resolveA, signIn } from "./helpers";

test.describe.configure({ mode: "serial" });

test("signs in and shows the live overview", async ({ page }) => {
  await signIn(page);
  await expect(page.getByRole("heading", { level: 1, name: "Network overview" })).toBeVisible();
  await expect(page.getByText("Resolver online")).toBeVisible();
});

test("rejects a wrong password", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("not the password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
});

test("creates a zone and record in the UI and answers them over DNS", async ({ page }) => {
  await signIn(page);
  await page.goto("/zones");
  await page.getByRole("button", { name: "Add zone" }).click();
  const zoneForm = page.getByRole("form", { name: "Create DNS zone" });
  await zoneForm.getByLabel("Zone name").fill("browser.test");
  await zoneForm.getByRole("button", { name: "Create zone" }).click();
  await expect(page.getByRole("heading", { level: 2, name: /browser\.test/ })).toBeVisible();

  await page.getByRole("button", { name: "Add record" }).first().click();
  const recordForm = page.getByRole("form", { name: "Add DNS record" });
  await recordForm.getByLabel(/Record name/).fill("www");
  await recordForm.getByLabel("IPv4 address").fill("192.0.2.80");
  await recordForm.getByRole("button", { name: "Save record" }).click();
  await expect(page.getByText("192.0.2.80")).toBeVisible();
  await expect.poll(() => resolveA("www.browser.test"), { timeout: 5000 }).toEqual(["192.0.2.80"]);
});

test("records DNS traffic in the query log", async ({ page }) => {
  await resolveA("www.browser.test");
  await signIn(page);
  await page.goto("/queries");
  await expect(page.getByText("www.browser.test.").first()).toBeVisible();
});

test("blocks configured domains", async () => {
  const resolver = await resolveA("ads.e2e.test");
  expect(resolver).toEqual([]);
});

test("finds a zone with the command palette", async ({ page }) => {
  await signIn(page);
  await page.keyboard.press("Control+K");
  const input = page.getByRole("combobox");
  await expect(input).toBeFocused();
  await input.fill("browser");
  await expect(page.getByRole("option", { name: /browser\.test/ }).first()).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/zones\?zone=/);
  await expect(page.getByRole("heading", { level: 2, name: /browser\.test/ })).toBeVisible();
});

test("rejects invalid settings with field errors", async ({ page }) => {
  await signIn(page);
  await page.goto("/settings");
  await page.getByLabel(/Allowed client networks/).fill("127.0.0.0/8, not-a-network");
  await page.getByRole("button", { name: "Save configuration" }).click();
  await expect(page.getByText(/Entry 2: invalid client network/)).toBeVisible();
  await expect(page.getByLabel(/Allowed client networks/)).toHaveAttribute("aria-invalid", "true");
});

test("keeps viewers read-only", async ({ page, request }) => {
  const csrf = await apiSignIn(request);
  const created = await request.post("/api/v1/users", {
    headers: { "X-CSRF-Token": csrf },
    data: { username: "viewer", password: "viewer password long", role: "viewer" },
  });
  expect([201, 409]).toContain(created.status());
  await signIn(page, "viewer", "viewer password long");
  await page.goto("/zones");
  await expect(page.getByRole("button", { name: "Add zone" })).toBeDisabled();
  await expect(page.getByText(/read-only/i).first()).toBeVisible();
});

test.describe("accessibility", () => {
  // axe is injected as an inline script, which the server's CSP rightly blocks.
  test.use({ bypassCSP: true });

  test("has no axe violations on key pages with real data", async ({ page }) => {
    const require = createRequire(import.meta.url);
    const axeSource = readFileSync(require.resolve("axe-core/axe.min.js"), "utf8");
    await signIn(page);
    for (const path of ["/", "/zones", "/queries", "/settings"]) {
      await page.goto(path);
      await page.locator("main").waitFor();
      await page.addScriptTag({ content: axeSource });
      const violations = await page.evaluate(async () => {
        const axe = (globalThis as unknown as { axe: { run: (context: Document, options: object) => Promise<{ violations: { id: string }[] }> } }).axe;
        const result = await axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } });
        return result.violations.map((violation) => violation.id);
      });
      expect(violations, `${path} axe violations`).toEqual([]);
    }
  });
});
