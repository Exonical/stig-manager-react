import { defineConfig, devices } from '@playwright/test'

/**
 * Default base URLs target the local compose stack. In CI we point at
 * the same containers; the workflow boots the stack and then runs
 * Playwright in this directory. Override `WEB_URL` / `API_URL` /
 * `KEYCLOAK_URL` from the environment to retarget without recompiling.
 */
const WEB_URL = process.env.WEB_URL ?? 'http://localhost:54000'
const API_URL = process.env.API_URL ?? 'http://localhost:54001'

export default defineConfig({
  testDir: 'tests',
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI
    ? [['list'], ['html', { open: 'never' }]]
    : [['list']],
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: WEB_URL,
    extraHTTPHeaders: { Accept: 'application/json' },
    trace: process.env.CI ? 'on-first-retry' : 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: process.env.CI ? 'retain-on-failure' : 'off',
    ignoreHTTPSErrors: true,
  },

  // The stack must already be up before tests run; see e2e/README.md
  // for the local recipe and .github/workflows/e2e.yaml for the CI
  // equivalent.
  projects: [
    {
      name: 'setup',
      testMatch: /.*\.setup\.ts/,
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        storageState: 'storage/admin.json',
      },
      dependencies: ['setup'],
      testIgnore: /.*\.setup\.ts/,
    },
  ],
  outputDir: 'test-results',
})

// Re-export the urls so individual specs can reach them without
// re-reading process.env.
export const urls = { web: WEB_URL, api: API_URL }
