import { defineConfig, devices } from "@playwright/test";

// Browser end-to-end tests against a real Velora DNS process (see
// e2e/start-server.mjs). Run with `npm run test:e2e` after `npm run build`.
const httpPort = Number(process.env.VELORA_E2E_HTTP_PORT ?? 18080);

export default defineConfig({
  testDir: "e2e",
  testMatch: "*.e2e.ts",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 30_000,
  reporter: process.env.CI ? [["list"], ["html", { open: "never", outputFolder: "e2e-report" }]] : "list",
  outputDir: "e2e-results",
  use: {
    baseURL: `http://127.0.0.1:${httpPort}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    locale: "en-US",
    timezoneId: "UTC",
    // Page transitions are cosmetic; keep assertions and the axe scan stable.
    contextOptions: { reducedMotion: "reduce" },
    launchOptions: process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {},
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] }, testIgnore: "mobile.e2e.ts" },
    { name: "phone", use: { ...devices["Pixel 7"] }, testMatch: "mobile.e2e.ts" },
  ],
  webServer: {
    command: "node e2e/start-server.mjs",
    url: `http://127.0.0.1:${httpPort}/ready`,
    timeout: 180_000,
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
  },
});
