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
