import { defineConfig } from '@playwright/test'

// pack-ui is given by the Go harness (PACK_UI_URL); one worker — the operator serializes the deploy itself.
export default defineConfig({
  testDir: './specs',
  timeout: 180_000,
  workers: 1,
  retries: 0,
  use: {
    baseURL: process.env.PACK_UI_URL,
    screenshot: 'only-on-failure',
    // the system's Google Chrome — no browser download (the CDN is unreachable from some networks)
    channel: 'chrome',
  },
})
