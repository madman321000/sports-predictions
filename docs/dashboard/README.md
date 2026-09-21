# Today's games and predictions

The website now shows **today's NBA/NFL games and pregame win probabilities**.
Today and game times use the browser's IANA timezone. Historical model reports,
season/player tables and training controls are no longer part of the website.
Player predictions are a later feature.

Training, backtesting, player data and the full PostgreSQL database stay local.
The Go API needs only small exported model JSON files. It automatically obtains
schedules and current-season team results from ESPN; no hosted PostgreSQL or
Python runtime is required.

## 1. Prepare local models

Keep using the [modeling workflow](../../modeling/README.md) to train and evaluate.
For an initial local check, export the existing baseline runs you already made:

```bash
source modeling/.venv/bin/activate
python -m modeling.export_model --run modeling/runs/nba-2026-v1 \
  --model-id nba-2026-v1 --out models/serving/nba.json
python -m modeling.export_model --run modeling/runs/nfl-2025-v1 \
  --model-id nfl-2025-v1 --out models/serving/nfl.json
```

These commands export the already-fitted scaler and logistic-regression weights;
they do not retrain or choose a model using test performance. Artifacts contain
feature names, means/scales, coefficients, intercept, model version and the last
validation date. They contain no player records, credentials or raw training data.
Only load `.joblib` files you trained locally: pickle/joblib is executable input.
The server reads JSON only and validates its schema and finite numeric weights.

For a selected candidate from a completed cross-season run instead:

```bash
python -m modeling.export_model --run modeling/runs/nba-cross-v1 \
  --candidate strength --model-id nba-strength-v1 --out models/serving/nba.json
```

Choose your version and candidate explicitly after local evaluation. The exporter
refuses overwrites: move the previous JSON to an ignored backup path before
exporting a replacement, then restart the API. Never overwrite your local run
artifacts. Go/Python inference parity is covered by tests.

## 2. Configure and start the API

Add/update these values in the root `.env` without replacing ingestion settings:

```dotenv
API_ADDRESS=127.0.0.1:8080
FRONTEND_ORIGIN=http://localhost:5173
FORECAST_MODEL_DIR=models/serving
ESPN_BASE_URL=https://site.api.espn.com
ESPN_REQUEST_INTERVAL=5s
ESPN_HTTP_TIMEOUT=20s
```

```bash
go run ./cmd/api
```

`DATABASE_URL`, `API_DATABASE_URL`, `STATS_DATABASE_URL` and `API_RUNS_DIR` are
**not used by this server**. Keep the ingestion database settings for local
training/data tools. Missing NBA/NFL artifacts allow schedule browsing with a
clear unavailable-prediction message; malformed artifacts fail startup.

## 3. Start the website

In another terminal:

```bash
cd frontend
npm ci
# Only if this file doesn't already exist:
cp .env.example .env
npm run dev
```

Open **http://localhost:5173**, matching `FRONTEND_ORIGIN`; `127.0.0.1:5173` is a
different browser origin. The frontend's `VITE_API_BASE_URL` remains
`http://localhost:8080`. No credentials go into `VITE_*` variables.

Select NBA or NFL. The UI checks every minute; server-side schedule caches are
shared across users/timezones and refreshed at most once per five minutes.
Refresh does not bypass the request limiter. Initial current-season history
loads in bounded date chunks in the background and can take a few minutes.
Only a fully successful backfill enables new predictions.

## Prediction rules and limitations

- Only regular-season games are predicted. ESPN preseason/All-Star events are
  excluded by the existing provider parser; postseason games can be displayed
  but do not get regular-season model predictions.
- Each team needs **five completed games in the current season**. Early-season
  NFL/NBA games can legitimately have no probability. Do not substitute invented
  50/50 predictions or silently reuse the previous season's team history.
- Features use strictly earlier **UTC dates**, matching local training. Browser
  timezone affects display and day selection, not feature cutoff semantics.
- A model is rejected for a game whose date overlaps its training/validation
  cutoff. Same-day outcomes and the target game's result never enter its features.
- New probabilities are generated only before the scheduled start and while the
  provider status is scheduled. After kickoff/tipoff, only a previously generated
  pregame prediction is shown; the API never reconstructs one from later data.
- Models and histories are separate: live results update feature inputs, never
  the fitted coefficients. Updating a model requires a new local export/restart.
- Histories refresh at most every five minutes while that league has visitors.
  Recent dates are rechecked for status changes/corrections. On failure, new
  predictions pause. Stale history older than 15 minutes is not used.
- The single ESPN client shares the existing five-second limiter and bounded
  retries. HTTP 403/429 halts provider requests for that process; fix the cause
  before restarting. Do not run a bulk local importer alongside this API: separate
  processes cannot share the limiter.
- Cache and pregame records are in memory. A restart needs a new history warmup
  and loses recorded pregame probabilities for already-started games. The page
  then shows them as unavailable. This is not a persistent prediction ledger.
- This is an experimental binary home-win model, not a tie model or a guarantee.
  Historical provider corrections and completeness remain limitations. No odds,
  injury adjustments, player predictions or betting recommendations are included.

## API

- `GET /api/health`: service status and number of installed league models.
- `GET /api/today?league=NBA&timezone=America%2FNew_York`: today's date, schedule
  timestamp, model ID, history status and games, each with either a probability
  pair or an explicit reason it is unavailable. `timezone` defaults to UTC.

Only NBA/NFL are accepted. Arbitrary date ranges cannot be requested by visitors.
The previous `/api/runs` and `/api/stats` routes are not mounted by `cmd/api`.
There are no training, model upload or credential endpoints.
