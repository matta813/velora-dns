import { Resolver } from "node:dns/promises";
import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const ADMIN = { username: "admin", password: "e2e admin password" };

export async function signIn(page: Page, username = ADMIN.username, password = ADMIN.password) {
  await page.goto("/");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  // The sign-in screen has its own heading; wait until the app replaced it.
  await expect(page.getByRole("button", { name: "Sign in" })).toBeHidden();
  await expect(page.locator("main h1")).toBeVisible();
}

/** Signs in through the API and returns the CSRF token for mutations. */
export async function apiSignIn(request: APIRequestContext, username = ADMIN.username, password = ADMIN.password) {
  const response = await request.post("/api/v1/auth/login", { data: { username, password } });
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: { csrf_token: string } }).data.csrf_token;
}

/** Resolves a name with a real DNS query against the test server. */
export async function resolveA(name: string): Promise<string[]> {
  const resolver = new Resolver({ timeout: 2000, tries: 1 });
  resolver.setServers([`127.0.0.1:${process.env.VELORA_E2E_DNS_PORT ?? "15353"}`]);
  try {
    return await resolver.resolve4(name);
  } catch {
    return [];
  }
}
