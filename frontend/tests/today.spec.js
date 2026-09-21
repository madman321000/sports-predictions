import { test, expect } from "@playwright/test";
test.use({ timezoneId: "America/Los_Angeles" });
test("today, browser timezone, predictions and league switching", async ({
  page,
}) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route("**/api/today?**", (route) => {
    const q = new URL(route.request().url()).searchParams;
    expect(q.get("timezone")).toBe("America/Los_Angeles");
    const league = q.get("league");
    return route.fulfill({
      headers: { "Access-Control-Allow-Origin": "*" },
      json: {
        league,
        date: "2026-10-28",
        model_id: "nba-v1",
        history_status: "ready",
        updated_at: "2026-10-28T18:00:00Z",
        games:
          league === "NFL"
            ? []
            : [
                {
                  id: "g1",
                  starts_at: "2026-10-29T02:00:00Z",
                  status: "scheduled",
                  home_team: "Home Team",
                  away_team: "Away Team",
                  prediction: {
                    home_probability: 0.65,
                    away_probability: 0.35,
                    model_id: "nba-v1",
                    generated_at: "2026-10-28T18:00:00Z",
                    home_history_games: 10,
                    away_history_games: 9,
                  },
                },
                {
                  id: "g2",
                  starts_at: "2026-10-28T16:00:00Z",
                  status: "final",
                  home_team: "Other Home",
                  away_team: "Other Away",
                  home_score: 110,
                  away_score: 100,
                  prediction: null,
                  unavailable: "No pregame prediction recorded.",
                },
              ],
      },
    });
  });
  await page.goto("/");
  await expect(page.getByText("65.0%", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "NBA", exact: true }).click();
  await expect(page.getByText("65.0%", { exact: true })).toBeVisible();
  await expect(page.getByText("7:00 PM", { exact: true })).toBeVisible();
  await expect(page.getByText("No pregame prediction recorded.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Sports data" })).toHaveCount(
    0,
  );
  await page.screenshot({
    path: "test-results/today-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: "test-results/today-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "NFL", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "No NFL games today" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
test("failure, retry and missing model states", async ({ page }) => {
  let failed = true;
  await page.route("**/api/today?**", (route) =>
    failed
      ? route.fulfill({
          status: 503,
          body: "Schedules unavailable",
          headers: { "Access-Control-Allow-Origin": "*" },
        })
      : route.fulfill({
          headers: { "Access-Control-Allow-Origin": "*" },
          json: {
            league: "NBA",
            date: "2026-10-28",
            history_status: "warming",
            updated_at: "2026-10-28T18:00:00Z",
            games: [],
          },
        }),
  );
  await page.goto("/");
  await expect(page.getByRole("alert")).toContainText("Schedules unavailable");
  failed = false;
  await page.getByRole("button", { name: "Refresh games" }).click();
  await expect(
    page.getByText("No NBA model installed.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("status")).toContainText("Preparing this season");
});
