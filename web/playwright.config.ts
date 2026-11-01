import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 45_000,
  expect: { timeout: 12_000 },
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // E2E_CHANNEL=msedge или chrome — запуск в установленном браузере без загрузки Chromium Playwright.
    channel: process.env.E2E_CHANNEL || undefined,
  },
  // Если задан E2E_BASE_URL, тесты идут против уже запущенного развёртывания (например, diary.cmd start), а dev-сервер не нужен.
  webServer: process.env.E2E_BASE_URL ? undefined : {
    command: 'npm run dev -- --host 127.0.0.1 --port 5173',
    url: 'http://127.0.0.1:5173',
    reuseExistingServer: true,
  },
})
