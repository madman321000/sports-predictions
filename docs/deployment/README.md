# Deployment is deferred until local prediction checks pass

The current architecture deploys **React + a Go inference API + exported model
JSON**. Training, evaluation, player records and the ingestion database remain
local. The previous Neon/publication-database setup is no longer required for
this website. Existing local databases and model runs are unchanged.

First follow the [local prediction guide](../dashboard/README.md). Verify:

1. NBA/NFL models export and load, and probabilities match local inference.
2. Today's games and times match your browser timezone.
3. Provider history refresh works on your network, including cold-start warmup.
4. Games without enough history clearly abstain, and started games never acquire
   an after-the-fact pregame prediction.
5. You are satisfied with local backtests before choosing production model versions.

No deployment is performed automatically by adding model files locally. The
workflow now requires **`FORECAST_DEPLOY_ENABLED=true`**, rather than the previous
`DEPLOY_ENABLED` variable, so the replacement API cannot deploy under old setup.
Leave the new flag absent/false until ready.

## Later hosting setup

The existing Vercel frontend and Render Docker workflow can be used later:

- Vercel receives only built static assets and the public API URL.
- Render runs the Go binary; it does not need a PostgreSQL connection or Python.
- Install trusted exported `nba.json` and `nfl.json` as Render secret files under
  `/etc/secrets`, matching `FORECAST_MODEL_DIR` in `render.yaml`. Model weights
  are not credentials, but do not need to be public repository files. The Docker
  context excludes local models, `.env`, datasets and raw reports.
- Configure `FRONTEND_ORIGIN` with the exact production frontend origin. Configure
  ESPN settings server-side as in the local guide; no browser-to-ESPN requests.
- Keep deployment tokens in GitHub's `production` environment secrets:
  `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`, `RENDER_DEPLOY_HOOK_URL`.
- Set the repository variable `VITE_API_BASE_URL` to the HTTPS API origin. Only
  after the model files and runtime are ready, set `FORECAST_DEPLOY_ENABLED=true`.

The workflow gates deployment on lint, tests, secret scanning and a container
build. Pull requests never deploy. The Render hook requests the tested commit;
check Render for completion afterward. Code deployment does not install or select
new model versions—manage those files deliberately after local evaluation.

Free backend restarts/cold starts discard in-memory histories and pregame records.
History will rebuild, but already-started games without a recorded pregame value
will show unavailable. A durable prediction ledger is a separate future task if
you need that reliability; do not represent this in-memory version as archival.
Public ESPN access and free-tier limits can change; validate on the chosen host
before enabling deployment. This PR does not create accounts, upload models,
configure production secrets or deploy anything.
