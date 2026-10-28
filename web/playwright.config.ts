import { defineConfig, devices } from "@playwright/test";

// End-to-end tests drive the real UI against the real services. Playwright
// starts its own copy of the stack, the way `make dev` does: topology serves
// the in-memory seed, so no Docker or Postgres is needed, and the numbers are
// the deterministic ones the Go tests pin down. The events service runs the
// simulator fast, so the live view shows events within seconds; tests of the
// sandbox open it with ?mode=sandbox.
//
// The ports are deliberately unusual so the tests never talk to something
// else that happens to be running, such as `make up` on :8080 or another
// project's Vite on :5173.
const port = { topology: 19001, impact: 19002, events: 19003, gateway: 18080, web: 15173 };

export default defineConfig({
  testDir: "e2e",
  testMatch: "*.e2e.ts",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  // Five processes and Vite's first compile share the machine when a run
  // starts, and the first analysis now waits for liveStatus to pick the
  // view. 5 s (the default) was occasionally too tight for the first test.
  expect: { timeout: 10_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: `http://localhost:${port.web}`,
    colorScheme: "light",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "go run ./cmd/topology",
      cwd: "..",
      env: { LISTEN_ADDR: `:${port.topology}` },
      port: port.topology,
      timeout: 120_000,
    },
    {
      command: "go run ./cmd/impact",
      cwd: "..",
      env: { LISTEN_ADDR: `:${port.impact}`, TOPOLOGY_ADDR: `localhost:${port.topology}` },
      port: port.impact,
      timeout: 120_000,
    },
    {
      command: "go run ./cmd/events",
      cwd: "..",
      env: {
        LISTEN_ADDR: `:${port.events}`,
        TOPOLOGY_ADDR: `localhost:${port.topology}`,
        LIGHTPATH_MODE: "live",
        SIM_INTERVAL: "2s",
        SIM_MEAN_REPAIR: "4s",
        SIM_MAX_CONCURRENT: "2",
      },
      port: port.events,
      timeout: 120_000,
    },
    {
      command: "go run ./cmd/gateway",
      cwd: "..",
      env: {
        LISTEN_ADDR: `:${port.gateway}`,
        TOPOLOGY_ADDR: `localhost:${port.topology}`,
        IMPACT_ADDR: `localhost:${port.impact}`,
        EVENTS_ADDR: `localhost:${port.events}`,
      },
      url: `http://localhost:${port.gateway}/healthz`,
      timeout: 120_000,
    },
    {
      command: `npm run dev -- --port ${port.web} --strictPort`,
      env: { GATEWAY_URL: `http://localhost:${port.gateway}` },
      url: `http://localhost:${port.web}`,
    },
  ],
});
