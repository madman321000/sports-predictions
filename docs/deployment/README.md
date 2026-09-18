# Free-tier deployment with GitHub Actions

Use Vercel Hobby for the React site, a Render free web service for the Go API,
and Neon Free for PostgreSQL. The repository includes the container, Render
blueprint and `.github/workflows/dashboard.yml`; creating the three hosted
projects and adding credentials is a one-time account setup.

Free tiers have limits: [Vercel Hobby](https://vercel.com/docs/plans/hobby) is for
personal, noncommercial use; [Render free services](https://render.com/docs/free)
sleep after inactivity and can have slow first requests; check current
[Neon limits](https://neon.com/pricing) before loading data. The default deployment stores
only published reports and historical predictions in Neon. To host the player
and season dataset too, follow the full-dataset section below and budget storage
for it. Training and ingestion can continue locally. No paid plan or always-on availability is assumed.

## 1. Create the database

Create a Neon Free project. Use its SQL editor to run
[`000006_create_dashboard_reports.up.sql`](../../migrations/000006_create_dashboard_reports.up.sql).
This table is independent of migrations 1–5, so a new publication-only database
needs only this migration. For an existing ingestion database, apply migration
6 after the previous migrations instead.

Create a dedicated `dashboard_reader` PostgreSQL role with LOGIN using the
provider's role management, then grant only:

```sql
GRANT CONNECT ON DATABASE neondb TO dashboard_reader;
GRANT USAGE ON SCHEMA public TO dashboard_reader;
GRANT SELECT ON public.dashboard_reports TO dashboard_reader;
```

Substitute your database/schema names if different. Keep the read-only role's
connection URL for Render, and the owner's URL for local migrations/publication.
Require TLS (`sslmode=require`); copy connection strings directly from Neon.
Never paste their actual values into source files, issues, PRs or Actions YAML.

Publish reports using the owner's connection string in your **ignored local**
`.env` under `API_DATABASE_URL` (or inject it from your password manager):

```bash
go run ./cmd/publish
```

This command reads `API_RUNS_DIR=published`. Use the
[dashboard guide](../dashboard/README.md) to generate public JSON first.
The publication command updates matching IDs transactionally; it does not delete
other published runs. Database rows persist across backend deployments.

## 2. Create the backend

In Render, create a Blueprint from this repository using `render.yaml`. Confirm
the free plan and `main` branch. In Render's managed environment settings set:

| Setting | Value |
| --- | --- |
| `API_DATABASE_URL` | Neon **read-only** role's TLS connection URL |
| `FRONTEND_ORIGIN` | Exact Vercel production origin, e.g. `https://your-site.vercel.app` (no trailing slash) |

The Docker image binds port 8080 and runs as a non-root user. Its build context
allows only Go API source and dependencies, excluding `.env`, reports, exports
and models. The API loads reports from Neon at startup, so redeploy/restart it
after publication. An empty publication table produces an empty dashboard.

Automatic Render Git deploys are disabled. Copy the service's Deploy Hook URL
from Settings into GitHub's secret store below. This URL is a credential. Do not
commit it. Render may perform an initial deployment during project creation;
later deployments are driven by Actions.

## 3. Create the frontend and secret store

Create a Vercel Hobby project with the **Other** framework preset and repository
root as its root directory. Do not configure a separate Git auto-deploy path;
the workflow uploads prebuilt static output. Record its project ID, organization
ID and a Vercel deployment token. Copy the production site origin into Render's
`FRONTEND_ORIGIN`.

In GitHub → Settings → Environments, create `production`, restricted to `main`.
Add these **environment secrets** using GitHub's encrypted secret UI:

| Secret | Source |
| --- | --- |
| `VERCEL_TOKEN` | Vercel account token authorized for this project |
| `VERCEL_ORG_ID` | Vercel organization/team ID |
| `VERCEL_PROJECT_ID` | Vercel project ID |
| `RENDER_DEPLOY_HOOK_URL` | Render service Settings → Deploy Hook |

In GitHub → Settings → Secrets and variables → Actions → **Variables**, set these
repository variables (not environment variables, because build jobs need them):

| Variable | Value |
| --- | --- |
| `VITE_API_BASE_URL` | Render HTTPS origin, e.g. `https://your-api.onrender.com` |
| `DEPLOY_ENABLED` | `true`, only after the settings above are ready |

The database credential lives in [Render's managed environment store](https://render.com/docs/configure-environment-variables).
Deployment tokens live in [GitHub environment secrets](https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions).
Neither is supplied to the React build. There is no browser-side private API key.
Only explicitly published data is public. Enable GitHub push protection/secret
scanning where available; if an actual credential is ever committed, revoke it
and rotate it before cleaning history.

## 4. Deploy and verify

After merge to `main`, Actions runs Go lint/race tests with PostgreSQL, Python
tests/lint, a container build, a redacted Git-history secret scan, React build
and browser tests. Only then can the production job receive secrets. PRs never
deploy. Production deployments are serialized.

Enable `DEPLOY_ENABLED`, then run **Dashboard CI and deployment → Run workflow**
on `main`, or push the next reviewed change. The job deploys the tested static
artifact using [Vercel's prebuilt deployment](https://vercel.com/docs/cli/deploy),
then requests a [Render deploy of that exact commit](https://render.com/docs/deploy-hooks).
A successful hook request means **accepted**, not that Render finished building.
Check Render's deployment status, then:

```bash
curl --fail https://YOUR-API.onrender.com/api/health
curl --fail https://YOUR-API.onrender.com/api/runs
```

Open the production dashboard, select a report, check its calibration chart and
page through predictions. First loads may wait for the free backend to wake up.
The frontend supports retry after network errors.

To pause deployments, set `DEPLOY_ENABLED=false`. To roll back, revert the
application commit through a PR and deploy it; database publications are separate
and are not rolled back by code deployments. Do not run the down migration to
roll back a frontend or API release—it deletes published data.

## Local verification

```bash
go test -race ./...
source modeling/.venv/bin/activate
python -m unittest discover -s modeling/tests -v
cd frontend
npm ci
npx playwright install chromium
npm test
npm run build
```

Set `TEST_DATABASE_URL` to a disposable PostgreSQL instance for integration
tests; tests use isolated schemas. See [development](../development/README.md).

## Hosting the full sports dataset later

The **Sports data** feature also needs the ingestion tables and views. A
publication-only database created with migration 6 is insufficient for it. Keep
`API_DATABASE_URL` for model publications and add **`STATS_DATABASE_URL`** in
Render's secret environment settings for a read-only connection to the full
sports database. Both URLs can point to the same database and reader role once
it contains all tables. Leaving `STATS_DATABASE_URL` empty disables only the
Sports data view. Do not add either URL to Vercel or `VITE_*` settings.

For a first deployment, restore a local snapshot into an **empty dedicated hosted
database**. Restoring copies schema and data, so do not apply migrations 1–5 first.
Create the dump using the PostgreSQL client in your current local container:

```bash
mkdir -p exports
docker compose exec -T db sh -c \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl -Fc' \
  > exports/sports-database.dump
```

The ignored `exports/` directory keeps the dump out of Git. Keep a backup locally.
For restore, use a PostgreSQL client at least as new as the source server. Configure
an ignored local `.pg_service.conf` entry named `sports_hosted` with the hosted
owner role's host, port, database, user and required TLS settings. Keep its
password in a password manager or an ignored `.pgpass` file with permissions
`chmod 600 .pgpass`; never commit real connection details. Then, from the repo:

```bash
PGSERVICEFILE="$PWD/.pg_service.conf" PGPASSFILE="$PWD/.pgpass" \
  pg_restore --dbname='service=sports_hosted' --no-owner --no-acl \
  --exit-on-error --single-transaction exports/sports-database.dump
```

Do not add `--clean` or restore into a database containing data you want to retain.
A snapshot includes the migration state present locally. If `dashboard_reports`
was not in the dump, apply migration 6 once afterward; otherwise do not rerun it.
Compare hosted table counts with local counts before using the hosted reader.

Grant the dedicated reader `CONNECT` on the database and `USAGE` on the schema,
then only these SELECT permissions (alongside `dashboard_reports` for models):

```sql
GRANT SELECT ON public.leagues, public.teams, public.games, public.players,
  public.player_game_stats, public.player_game_imports,
  public.player_complete_games, public.player_season_totals
TO dashboard_reader;
```

Ingestion and training can continue locally; they do not automatically synchronize
with the hosted snapshot. Refreshing hosted raw data needs an explicit later
import/restore workflow. `cmd/publish` updates model reports only. API stats
queries read committed database state on each request; model reports still load
at startup. Monitor database storage as more seasons are added, including indexes.
The deployment workflow deploys code only and never migrates or overwrites data.
