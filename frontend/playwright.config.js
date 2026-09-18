import { defineConfig } from "@playwright/test";
const port = process.env.PLAYWRIGHT_PORT || "5173";
export default defineConfig({
  testDir: "tests",
  use: { baseURL: `http://localhost:${port}` },
  webServer: {
    command: `npm run dev -- --port ${port} --strictPort`,
    url: `http://localhost:${port}`,
    reuseExistingServer: !process.env.CI,
  },
});
