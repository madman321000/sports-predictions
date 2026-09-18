# Dashboard and read-only API

The React dashboard displays published model metrics, calibration curves and
historical game predictions. These are development backtests, not upcoming-game
forecasts. Training and ESPN ingestion continue to run locally.

## Publish existing evaluations

From the repository root, activate the modeling environment and create a separate
public report. The publisher copies only dashboard fields; models, local paths,
input hashes and raw export files stay local.

```bash
source modeling/.venv/bin/activate
mkdir -p published
python -m modeling.publish --run modeling/runs/nba-2025-eval-v1 --out published/nba-2025.json
python -m modeling.publish --run modeling/runs/nfl-2024-eval-v1 --out published/nfl-2024.json
```

Inspect these JSON files before uploading them: their selected contents will be
public. Use a new filename to preserve older publications, or explicitly remove
an old generated JSON before replacing it. The publisher refuses accidental
file overwrites. Walk-forward reports show game IDs; cross-season reports also
include ESPN team IDs and final scores.

## Run locally

Add these settings to your existing root `.env` (do not replace your ingestion
settings). Leave the database URL empty to serve the local `published` directory:

```dotenv
API_ADDRESS=127.0.0.1:8080
API_RUNS_DIR=published
API_DATABASE_URL=
FRONTEND_ORIGIN=http://localhost:5173
```

Start the API:

```bash
go run ./cmd/api
```

In another terminal, install Node.js 24+, then:

```bash
cd frontend
npm ci
cp .env.example .env
npm run dev
```

Open <http://localhost:5173>. `VITE_API_BASE_URL` is a **public** URL, never a
password or token. Every `VITE_*` variable used by the frontend becomes visible
to visitors. Restart the API after publishing or updating reports; it serves an
immutable snapshot loaded at startup.

## API contract

All endpoints are public, read-only JSON. There is no ingestion, training,
upload, filesystem download or database administration endpoint.

| Method and path | Response |
| --- | --- |
| `GET /api/health` | Status and number of loaded runs |
| `GET /api/runs` | Run IDs, titles, league, kind and generation time |
| `GET /api/runs/{id}` | Metrics, calibration, season and limitations; no prediction rows |
| `GET /api/runs/{id}/predictions` | `{total, offset, limit, items}` |

Predictions accept `candidate`, `offset` (default 0) and `limit` (default 50,
maximum 200). Unknown runs return 404; invalid pagination/candidates return 400.
CORS allows exactly `FRONTEND_ORIGIN`; CORS does not make this public API private.
Publication IDs use letters, digits, underscores and hyphens, up to 80 characters.
Only explicit `.json` reports are loaded, with a 32 MiB limit per report.

For hosted PostgreSQL, apply migration `000006`, set `API_DATABASE_URL`, then run
`go run ./cmd/publish`. This validates every report first and atomically upserts
reports by filename into `dashboard_reports`. Reports absent from the directory
are retained. The API only reads this table; it does not expose the raw ingestion
database. See [deployment](../deployment/README.md) for roles and secrets.

## Cross-season evaluation

Freeze the scaler, coefficients and C selection using the earlier season, then
backtest the later season without retraining. Candidate sets remain those in
`modeling/protocol.py`. The earliest 80% of eligible earlier-season dates (NBA)
or weeks (NFL) train the model; the remaining groups select C. There is no refit.

```bash
python -m modeling.cross_season --earlier exports/nba-2025 --later exports/nba-2026 --out modeling/runs/nba-cross-v1
python -m modeling.cross_season --earlier exports/nfl-2024 --later exports/nfl-2025 --out modeling/runs/nfl-cross-v1
python -m modeling.publish --run modeling/runs/nba-cross-v1 --out published/nba-cross-v1.json
python -m modeling.publish --run modeling/runs/nfl-cross-v1 --out published/nfl-cross-v1.json
```

Later-season team history starts empty, uses only strictly earlier UTC dates and
requires five prior games. Earlier exports need NFL week metadata. Both seasons
have already been inspected, so this is still development evaluation. Keep the
reserved future holdout untouched. `dashboard.json` retains local provenance and
saved `.joblib` files are local trusted artifacts, never web downloads.

## Browse the full sports database

Choose **Sports data** in the dashboard to browse leagues, seasons, teams, games,
players, player box scores and additive season totals. This reads the ingestion
database directly; it does not call ESPN or run ingestion when you open a page.

For your existing local database (migrations 1–5 already applied):

1. Add `STATS_DATABASE_URL` to the root `.env`. Set it to the same connection URL
   as your existing `DATABASE_URL`, or preferably a dedicated reader URL. Copy the
   actual URL value; `.env` variable interpolation is not supported. Keep
   `API_DATABASE_URL` empty to continue loading model reports from `published/`.
2. Restart `go run ./cmd/api` from the repository root. The stats connection pool
   is limited to four connections and defaults to read-only transactions.
3. Start/restart the frontend with `cd frontend && npm run dev`.
4. Open **http://localhost:5173**, matching `FRONTEND_ORIGIN`. Opening
   `http://127.0.0.1:5173` instead causes a CORS error with this configuration.
5. Select **Sports data**, a league, season and season type. Use the Browse
   selector for games, teams, players, box scores or totals. From Games, choose
   **View box score**. From Players, search a name and choose **Player games** or
   **Player totals**. Clear the selection to return to all players/games.

No additional migration or reimport is needed for this feature. With
`STATS_DATABASE_URL` empty, model results still work and Sports data shows setup
instructions. Use an existing or empty `published/` directory for the local API.
Player tables show only complete imported final-game box scores. DNP rows are
labeled; blank/unknown provider values stay unchanged. A player's team membership
reflects observed box scores, not a current roster. Season totals exclude DNPs,
keep traded players' team totals separate, and sum only supported additive
metrics—never percentages or ratings. Counts describe imported data only.

### Statistics API

All routes below return `{total, limit, offset, items}`. IDs used in filters are
local database IDs serialized as strings, distinct from `external_id` (ESPN).
All routes are read-only and public when deployed. They expose sports data only.

| GET endpoint | Required filters | Optional filters |
| --- | --- | --- |
| `/api/stats/leagues` | None | Pagination |
| `/api/stats/seasons` | `league` | Pagination |
| `/api/stats/teams` | `league`, `season` | `season_type`, pagination |
| `/api/stats/games` | `league`, `season` | `season_type`, `team_id`, `status`, pagination |
| `/api/stats/players` | `league`, `season` | `season_type`, `team_id`, `q` (name substring), pagination |
| `/api/stats/player-games` | `league`, `season` | `season_type`, `team_id`, `player_id`, `game_id`, `category`, pagination |
| `/api/stats/totals` | `league`, `season` | `season_type`, `team_id`, `player_id`, `category`, pagination |

`league` is NBA or NFL. The provider is ESPN. `season_type` defaults to 2 (regular
season); 1 is preseason and 3 is postseason. Pagination uses `limit` (default 25,
maximum 100) and `offset` (default 0, maximum 1,000,000). Unsupported filters,
duplicate query parameters and invalid values return 400. Empty/out-of-range
pages return 200 with an empty items array. Database failures return a generic
503 without credentials or SQL details. Queries have a 10-second deadline.
Totals are one row per player/team/category/metric, with `games_with_metric`
showing the number of complete games contributing that metric.

```bash
curl 'http://127.0.0.1:8080/api/stats/seasons?league=NBA'
curl 'http://127.0.0.1:8080/api/stats/games?league=NBA&season=2026&limit=25'
curl 'http://127.0.0.1:8080/api/stats/players?league=NFL&season=2025&q=Allen'
```

Team and season dropdowns show up to 100 options; their API lists are paginated.
All result tables are paginated. Unfinished/postponed game rows remain visible
with their stored status; this browser does not replace export quality checks.
