import { test, expect } from "@playwright/test";
const candidate = {
  metrics: { games: 30, accuracy: 0.7, log_loss: 0.6, brier_score: 0.2 },
  calibration: [
    {
      games: 30,
      lower: 0.5,
      upper: 0.6,
      mean_probability: 0.55,
      observed_home_win_rate: 0.7,
    },
  ],
};
test("dashboard filters, pagination, calibration and mobile layout", async ({
  page,
}) => {
  await page.route("**/api/**", (route) => {
    const url = new URL(route.request().url());
    let body;
    if (url.pathname === "/api/runs")
      body = [{ id: "nba", title: "NBA 2025 → 2026", league: "NBA" }];
    else if (url.pathname.endsWith("/predictions")) {
      let offset = Number(url.searchParams.get("offset"));
      body = {
        total: 30,
        items: [
          {
            game_id: `game-${offset}`,
            date: "2026-01-01",
            probability: 0.7,
            home_win: 1,
            home_team_id: "1",
            away_team_id: "2",
            home_score: 110,
            away_score: 100,
          },
        ],
      };
    } else
      body = {
        league: "NBA",
        training_season: 2025,
        test_season: 2026,
        candidates: { strength: candidate, strength_recent: candidate },
        limitations: ["Historical backtest only"],
      };
    return route.fulfill({
      json: body,
      headers: { "Access-Control-Allow-Origin": "*" },
    });
  });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Historical games" }),
  ).toBeVisible();
  await expect(page.getByText("game-0", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Next" }).click();
  await expect(page.getByText("game-25", { exact: false })).toBeVisible();
  await page.getByLabel("Candidate").selectOption("strength_recent");
  await expect(page.getByText("Page 1")).toBeVisible();
  await expect(page.getByRole("img")).toBeVisible();
  await page.screenshot({
    path: "test-results/dashboard-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByRole("heading", { name: "Historical games" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: "test-results/dashboard-mobile.png",
    fullPage: true,
  });
});
test("error and retry", async ({ page }) => {
  let failed = true;
  await page.route("**/api/**", (r) =>
    failed
      ? r.fulfill({
          status: 503,
          body: "offline",
          headers: { "Access-Control-Allow-Origin": "*" },
        })
      : r.fulfill({
          json: [],
          headers: { "Access-Control-Allow-Origin": "*" },
        }),
  );
  await page.goto("/");
  await expect(page.getByRole("alert")).toBeVisible();
  failed = false;
  await page.getByRole("button", { name: "Retry" }).click();
  await expect(
    page.getByText("No published runs yet.", { exact: false }),
  ).toBeVisible();
});
