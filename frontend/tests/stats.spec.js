import { test, expect } from "@playwright/test";

test("browse seasons, filters, player details and totals", async ({ page }) => {
  const requests = [];
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.route("**/api/**", (route) => {
    const u = new URL(route.request().url());
    requests.push(u);
    let items = [],
      total;
    switch (u.pathname) {
      case "/api/runs":
        return route.fulfill({
          json: [],
          headers: { "Access-Control-Allow-Origin": "*" },
        });
      case "/api/stats/leagues":
        items = [{ abbreviation: "NBA" }, { abbreviation: "NFL" }];
        break;
      case "/api/stats/seasons":
        items =
          u.searchParams.get("league") === "NFL"
            ? []
            : [{ season: 2026, season_type: 2, games: 30, final_games: 29 }];
        break;
      case "/api/stats/teams":
        items = [
          {
            id: "1",
            name: "Home Team",
            abbreviation: "HT",
            external_id: "100",
          },
        ];
        break;
      case "/api/stats/games":
        items = [
          {
            id: "20",
            starts_at: "2026-01-01T00:00:00Z",
            home_team: "Home Team",
            away_team: "Away Team",
            home_score: 110,
            away_score: 100,
            status: "final",
            player_stats_complete: true,
          },
        ];
        total = 30;
        break;
      case "/api/stats/players":
        items = [
          {
            id: "30",
            name: "Test Player",
            external_id: "p1",
            teams: "Home Team",
            games: 10,
          },
        ];
        break;
      case "/api/stats/player-games":
        items = [
          {
            player: "Test Player",
            team: "Home Team",
            game_external_id: "game1",
            starts_at: "2026-01-01T00:00:00Z",
            category: "general",
            stats: { points: "22", minutes: "--" },
            did_not_play: false,
            starter: null,
          },
        ];
        break;
      case "/api/stats/totals":
        items = [
          {
            player: "Test Player",
            team: "Home Team",
            category: "general",
            metric: "points",
            total: 220,
            games_with_metric: 10,
          },
        ];
        break;
    }
    return route.fulfill({
      json: { items, total: total ?? items.length },
      headers: { "Access-Control-Allow-Origin": "*" },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Sports data", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "NBA 2026 coverage" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "View box score" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Next data" }).click();
  await expect
    .poll(() =>
      requests.some(
        (u) =>
          u.pathname.endsWith("/games") &&
          u.searchParams.get("offset") === "25",
      ),
    )
    .toBeTruthy();
  await page.getByRole("button", { name: "View box score" }).click();
  await expect(
    page.getByText("Game: Home Team vs Away Team", { exact: false }),
  ).toBeVisible();
  await page.getByText("View statistics", { exact: true }).click();
  await expect(page.getByText("minutes", { exact: true })).toBeVisible();
  expect(
    requests.some(
      (u) =>
        u.pathname.endsWith("/player-games") &&
        u.searchParams.get("game_id") === "20",
    ),
  ).toBeTruthy();
  await page
    .getByRole("combobox", { name: "Browse", exact: true })
    .selectOption("players");
  await page.getByLabel("Player name", { exact: true }).fill("Test");
  await expect
    .poll(() =>
      requests.some(
        (u) =>
          u.pathname.endsWith("/players") && u.searchParams.get("q") === "Test",
      ),
    )
    .toBeTruthy();
  await page
    .getByRole("button", { name: "Player totals", exact: true })
    .click();
  await expect(
    page.getByRole("cell", { name: "220", exact: true }),
  ).toBeVisible();
  expect(
    requests.some(
      (u) =>
        u.pathname.endsWith("/totals") &&
        u.searchParams.get("player_id") === "30",
    ),
  ).toBeTruthy();
  await page
    .getByRole("combobox", { name: "Team", exact: true })
    .selectOption("1");
  await expect
    .poll(() =>
      requests.some(
        (u) =>
          u.pathname.endsWith("/totals") &&
          u.searchParams.get("team_id") === "1",
      ),
    )
    .toBeTruthy();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: "test-results/stats-mobile.png",
    fullPage: true,
  });
  await page
    .getByRole("combobox", { name: "League", exact: true })
    .selectOption("NFL");
  await expect(
    page.getByText("No imported seasons for this league."),
  ).toBeVisible();
  await expect(
    page.getByText("Player: Test Player", { exact: false }),
  ).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("disabled statistics explains setup without breaking model view", async ({
  page,
}) => {
  await page.route("**/api/**", (route) =>
    route.request().url().includes("/api/stats/")
      ? route.fulfill({
          status: 503,
          body: "Statistics browsing is not configured. Set STATS_DATABASE_URL and restart the API.",
          headers: { "Access-Control-Allow-Origin": "*" },
        })
      : route.fulfill({
          json: [],
          headers: { "Access-Control-Allow-Origin": "*" },
        }),
  );
  await page.goto("/");
  await page.getByRole("button", { name: "Sports data", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("STATS_DATABASE_URL");
  await page
    .getByRole("button", { name: "Model results", exact: true })
    .click();
  await expect(
    page.getByText("No published runs yet.", { exact: false }),
  ).toBeVisible();
});
