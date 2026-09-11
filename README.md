# sports-predictions

A Go learning project for a sports prediction platform. It seeds NBA/NFL league
reference data and imports NBA/NFL teams, schedules, game results, and historical player box scores from ESPN
into PostgreSQL. Prediction models are not implemented yet.

## Code organization

- `cmd/`: configuration, dependency wiring, and command lifecycle.
- `internal/seed/` and `internal/ingest/`: workflows and the interfaces they consume.
- `internal/league/`, `internal/team/`, `internal/game/`, and `internal/player/`: shared application types.
- `internal/postgres/`: repositories and SQL (`PostgresLeagueRepository` and
  `PostgresTeamRepository`, plus `PostgresGameRepository` and `PostgresPlayerRepository`).
- `internal/provider/espn/`: one package split by responsibility: `client.go`
  executes HTTP requests, `rate_limit.go` handles pacing and cancellation,
  `retry.go` contains retry policy, `errors.go` defines provider errors, and
  `teams.go`, `games.go`, and `players.go` fetch and decode teams, scoreboards, and player box scores for both sports.
  `leagues.go` selects the endpoint; `status.go` maps game statuses. Tests follow the same file grouping.

The workflows call `FetchTeams`, `FetchGames`, or `FetchPlayerGame` on their source
and persist through store interfaces. The command wires `IngestTeams`,
`IngestGames`, and `IngestPlayers`; ESPN response
types stay inside the provider package. `options.go` validates CLI arguments,
while game-date and player-game workers live in their corresponding ingest files.

## Prerequisites

- Go 1.27.1 (the version declared in `go.mod`).
- Docker with Docker Compose available and the Docker engine running.
- Git to clone the repository.
- Local port 5432 available for PostgreSQL.

Run the following commands from the repository root in a POSIX shell such as
Bash or Zsh. The database credentials below match `docker-compose.yml` and are
for local development.

## Run locally

### 1. Get the code and dependencies

```sh
git clone https://github.com/madman321000/sports-predictions.git
cd sports-predictions
go mod download
```

If you already have a checkout, use that directory instead. To try an unmerged
PR, check out its branch before running the commands below.

### 2. Configure your environment

```sh
cp .env.example .env
```

Edit `.env` for your local setup. It holds PostgreSQL image, credentials, database
name and host port; application and test connection URLs; ESPN's base URL; request
spacing, timeouts, and `INGEST_WORKERS` (defaults to 1). Keep the connection URLs in sync when changing the database
settings. URLs are explicit: do not use variable interpolation in the Go settings.

`.env` and local `.env.*` files are ignored by Git. Commit only the sanitized
`.env.example` template. Run commands from the repository root so both Go and
Docker Compose find `.env`. Exported environment variables override file values;
a missing file is allowed when all required settings are supplied externally.
The seed command requires `DATABASE_URL`; ingestion also requires `ESPN_BASE_URL`.

### 3. Start PostgreSQL

```sh
docker compose up -d --wait --wait-timeout 60 db
```

Compose waits for the database health check before returning. Inspect startup
problems with `docker compose logs db`.

### 4. Apply the migrations

On a fresh database, run:

```sh
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' \
  < migrations/000001_create_leagues.up.sql
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' \
  < migrations/000002_create_teams.up.sql
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' \
  < migrations/000003_create_games.up.sql
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' \
  < migrations/000004_create_import_snapshots.up.sql
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 --single-transaction' \
  < migrations/000005_create_players.up.sql
```

This uses the PostgreSQL client inside the container; no local `psql` installation
is required. The project does not yet have a migration runner or migration history
table. Apply each migration once per database, in numeric order. If you already
applied migrations 000001 through 000003, run only
`000004_create_import_snapshots.up.sql` followed by `000005_create_players.up.sql`.
If migrations 000001 through 000004 are already applied, run only migration 000005.
Skip migrations already applied; these migrations are not designed to be rerun.

### 5. Seed NBA and NFL

```sh
go run ./cmd/seed
```

A successful run logs `Seeded Leagues`. Seeding can be rerun: existing leagues
are updated by abbreviation instead of duplicated.

Inspect the records:

```sh
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
SELECT id, name, abbreviation, sport FROM leagues ORDER BY abbreviation;
SQL
```

Expect two rows: NBA and NFL.

### 6. Import NBA and NFL teams

```sh
go run ./cmd/ingest -resource teams -league NBA
go run ./cmd/ingest -resource teams -league NFL
```

Each command checks that its league has been seeded and whether a complete team
import is already stored. It skips ESPN when that import is complete; otherwise
it fetches the entire team list in one request and saves it in a transaction. Teams are identified by provider,
league, and external ID, so overlapping NBA/NFL IDs do not collide. Explicit refreshes update
existing records; missing teams are not deleted. `-resource teams` remains the
default for compatibility with the previous command.

### 7. Import games for a date range

Import the corresponding teams first, then run:

```sh
go run ./cmd/ingest -resource games -league NBA -from 2026-01-01 -to 2026-01-07
go run ./cmd/ingest -resource games -league NFL -from 2025-09-07 -to 2025-09-08
```

Both dates are required and inclusive. Full-season and longer date ranges are supported. They select
ESPN scoreboard calendar dates, not a filter on UTC game start times. Start times
are stored in UTC. NFL season year, season type, and week are preserved when
provided; the season year is not inferred from the game's calendar year.

Preseason events (`season.type=1`) and All-Star exhibitions
(`competition.type.abbreviation=ALLSTAR`) are explicitly excluded and logged.
ESPN can mark All-Star events as regular season, so both fields are checked.
Unknown teams in other games still fail the date rather than being silently
skipped or added as fake league teams. Mixed dates save only included games in
the date import record. Exhibition-only dates record an empty successful import;
like other empty dates, they are fetched again on a later run and do not create
missing-game findings in the quality report. No new migration is required.

Scheduled games have null scores, while a played score of zero is preserved.
Imports handle in-progress, final, postponed, canceled, suspended, and delayed
games. Unknown statuses and malformed records fail that date rather than silently
being skipped. No-game days are valid and count as completed dates.

Game upserts preserve identity and update start times, status, and scores. Each
date is committed atomically. A missing team fails the batch with an instruction
to import teams; no placeholder teams are created. Older fetched snapshots cannot
overwrite newer ones when workers finish out of order. Missing games are not
deleted, so refresh the relevant dates to collect later results or schedule changes.

### 8. Import historical player data

After importing teams and **all game dates you want to analyze**, run:

```sh
# NBA 2025–26 uses ESPN season year 2026; NFL 2025 uses year 2025.
go run ./cmd/ingest -resource players -league NBA -season 2026
go run ./cmd/ingest -resource players -league NFL -season 2025

# Postseason is a separate dataset; regular season (2) is the default.
go run ./cmd/ingest -resource players -league NBA -season 2026 -season-type 3
go run ./cmd/ingest -resource players -league NFL -season 2025 -season-type 3
```

Set `INGEST_TIMEOUT=4h` in your existing `.env` for a full-season backfill. At
five seconds per request, NBA player box scores alone take roughly two hours for
a full regular season; NFL takes roughly 23 minutes, plus response/processing time.
The five-minute default is useful for small runs but will interrupt a season import.
Rerunning resumes from committed games. Keep only one importer running at a time.
`-workers 1` through `4` shares the existing rate limiter; `-force` refreshes stored
box scores to collect corrections. Neither flag bypasses pacing or blocking responses.

The command selects **stored final games** for the requested league, season and
season type. It makes no scoreboard or team-list requests. Each missing game needs
one ESPN summary request, which contains both teams' player statistics. The
response must match the requested game, league, season and teams and be final.
An empty or malformed summary fails without marking that game complete.

Migration 000005 adds:

- `players`: provider/league-scoped ESPN player ID and display name.
- `player_game_stats`: historical team, position and jersey when provided,
  nullable starter flag, explicit did-not-play flag, category and raw statistic
  values. NFL players may have multiple categories in one game.
- `player_game_imports`: atomic per-game completion records. Missing statistic
  rows invalidate completion; failed refreshes retain the previous complete import.
- `player_season_teams`: team membership observed in imported game box scores,
  preserving trades instead of assigning every historical game to a current team.
- `player_season_totals`: additive totals per player, team, season, season type,
  category and metric, derived from complete imported games. Made/attempted pairs
  are split into numeric metrics. Percentages, averages, ratings, minutes and
  longest plays are retained as raw values rather than incorrectly summed.
- `player_season_coverage`: stored final games versus completed player imports.

These are **basic player profiles and box-score participation history**, not a
complete historical roster or biography dataset. In particular, NFL box scores
omit players without recorded statistics. Missing fields and DNP values are not
zero statistics. Raw values preserve formats such as `17/32` and `--`.
Statless DNP entries without an ESPN player ID are excluded and counted in the
import summary; no identity is guessed from a short name. Identified DNP entries
remain stored. A row with statistics or participation still requires an ID and
name, and conflicting duplicate categories still fail the import. The excluded
DNP count describes the current run and is not persisted as a roster record.

Season totals cover only imported games; they are not independently fetched ESPN
season totals. Importing seven days of games produces seven days of player data,
not a full season. Coverage checks cannot discover dates absent from your database.
To backfill the most recent completed seasons, run one range per league before
the player commands above. Set `INGEST_TIMEOUT=4h` in your existing `.env` first:

```sh
go run ./cmd/ingest -resource games -league NBA \
  -from 2025-10-01 -to 2026-06-30 -workers 4

go run ./cmd/ingest -resource games -league NFL \
  -from 2025-09-01 -to 2026-02-28 -workers 4
```

Dates flow through a queue bounded by the worker count, so a long range does not
need to fit in memory before workers start. Progress logs show finished dates,
imported dates, skipped dates and processed game records after every ten finished
dates, after a date finishes at least 30 seconds since the last update, and on the
last date. The final summary also reports partial progress on failure. Dates may
finish out of order. Rerun the same range to resume; completed final dates are
skipped, while empty or unfinished dates are checked again.

At five-second request spacing, the NBA example needs roughly 23 minutes for
scoreboard requests alone and NFL roughly 15 minutes, plus response time and
retries. The default five-minute command timeout is too short for a fresh season.
More workers overlap processing and database writes; all ESPN requests still
share the existing limiter. Run the league commands sequentially, not as separate
concurrent processes.

Inspect coverage and totals using the database container:

```sh
docker compose exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
SELECT * FROM player_season_coverage ORDER BY league_id, season, season_type;
SELECT p.name, l.abbreviation, t.abbreviation AS team, s.season, s.season_type,
       s.category, s.metric, s.total, s.games_with_metric
FROM player_season_totals s
JOIN players p ON p.id=s.player_id
JOIN leagues l ON l.id=s.league_id
JOIN teams t ON t.id=s.team_id
ORDER BY p.name, s.season, s.category, s.metric;
SQL
```

Before modeling, check date coverage and resolve failed imports. Features for a
particular game must use only earlier games; using end-of-season totals to predict
an earlier game would leak its future results into the model.

### Avoid repeat requests

Normal imports check PostgreSQL before contacting ESPN:

- **Teams:** skip the request when a successful full team import exists and every
  team in that response is still in the database. This applies to both NBA and NFL.
- **Games:** skip a date when a successful full scoreboard import exists and every
  game in that response is stored as final with both scores. Zero is a valid score.
- **Players:** skip a final game with an intact player import; use `-force` for corrections.
- Dates with scheduled, live, postponed, canceled, or missing games still refresh.
  Empty dates also refresh so a previously empty future schedule is not cached forever.

ESPN returns a whole scoreboard date, so a mixed date with unfinished games still
needs one request even when some of its games are already final. Completion is
tracked using the requested ESPN date, not the games' UTC start dates.

Migration 000004 adds persistent import records, saved in the same transaction as
their teams or games. Existing rows from before this change require one verification
import to establish a full response; partial rows alone cannot prove completeness.
A failed import does not create a completion record. Deleted records invalidate
completion and cause the next run to fetch again.

Use `-force` to refresh saved teams, discover schedule changes, or collect corrected
final scores:

```sh
go run ./cmd/ingest -resource teams -league NFL -force
go run ./cmd/ingest -resource games -league NBA -from 2026-01-01 -to 2026-01-07 -force
```

Completed imports do not expire automatically. The CLI reports skipped teams or
skipped dates separately from newly processed records. Rate limits still apply to
forced requests. Concurrent processes are not deduplicated; continue to run only
one importer process at a time.

### Bounded concurrency

The default is one date worker. To overlap date processing and database writes:

```sh
go run ./cmd/ingest -resource games -league NFL -from 2025-09-07 -to 2025-09-14 -workers 3
```

Set `INGEST_WORKERS` in `.env` for a persistent default (1-4), or override it with
`-workers`. Every worker shares the same ESPN client: HTTP requests remain
serialized and at least five seconds apart, including retries. More workers do
not increase ESPN request throughput or bypass access restrictions.

On the first error, pending workers are canceled. Dates already committed remain;
the command reports committed dates, skipped dates, and processed game records even on failure.
Counts are records processed, not unique inserts (a rescheduled game can appear on
multiple dates). Rerun the same date range to recover. Completed import records persist in PostgreSQL; there is no cross-process limiter yet; run only one importer process at a time.

Set `ESPN_REQUEST_INTERVAL=10s` in `.env` for a longer delay. The optional
`-request-interval` flag overrides that setting for a single run. The five-second
minimum remains enforced regardless of where the value comes from.

### ESPN request limits

These are conservative application defaults, **not a published ESPN allowance or
a guarantee against blocking**. ESPN's [terms page](https://support.espn.com/hc/en-us/articles/360035445091-Terms-of-Use)
links to the applicable [terms of use](https://disneytermsofuse.com/). Endpoint
availability does not establish permission for every use of its data.

- One in-flight request per shared client, at least five seconds apart, including
  retries. The interval can be increased up to one hour, but cannot be set below
  five seconds.
- At most three attempts total, and only HTTP 502, 503, and 504 are retried.
  Backoff doubles between retries and respects longer `Retry-After` values
  expressed as seconds or an HTTP date. Invalid or more-than-one-minute server
  delays stop the client for its lifetime instead of retrying early.
- HTTP 403 or 429 stops the client for the rest of its lifetime. The error includes
  `Retry-After` if provided. Do not repeatedly restart the command after a block;
  wait at least the requested delay and resolve access restrictions before retrying.
- Other HTTP errors, network errors, and invalid response data fail without retries.
  Redirects are not followed. Requests default to a 20-second timeout (`ESPN_HTTP_TIMEOUT`); the command
  defaults to a five-minute deadline (`INGEST_TIMEOUT`) and supports cancellation with Ctrl+C.
- There is no automatic polling or per-team fan-out. Game imports use one
  scoreboard request per selected date. Team reference data changes
  infrequently: run manually when needed, rather than on a frequent schedule.
- The limiter is per client/process and resets on restart. Run only one importer
  at a time. Multiple machines or processes would need a shared limiter before
  adding scheduled jobs or concurrency across processes.

Tests use a local HTTP server and a small synthetic ESPN-shaped fixture. CI never
calls the live ESPN endpoint.

## Check data quality and export

After applying migrations through **000005** and importing the games and players,
use the offline data command. This change requires **no new migration** and makes
no ESPN requests. It reads `DATABASE_URL` from your existing `.env`; ESPN settings
are not required. Queries run in a read-only, repeatable database snapshot.

```sh
go run ./cmd/data -action quality -league NBA -season 2026 \
  -from 2025-10-01 -to 2026-06-30

go run ./cmd/data -action quality -league NFL -season 2025 \
  -from 2025-09-01 -to 2026-02-28
```

The command prints a JSON report to stdout and returns a nonzero exit status when
it finds problems. Redirect stdout to a file if you want to save the report;
errors remain on stderr. The report lists:

- Stored and final games, games with intact player imports, and player-category rows.
- Non-final games, missing final scores, missing/incomplete player imports, and
  empty or missing statistic values for players who played. Zero is valid; DNP
  rows do not need statistics. Optional position, jersey and starter fields are
  allowed to be missing.
- Games listed in saved imports for the expected date range but missing from
  the games table. These findings cover the date range across season types, since
  a deleted row no longer has season metadata.
- Expected dates without a successful scoreboard import, including dates that
  have no games. Omitting `-from`/`-to` produces an unchecked-date finding.

`-season-type 2` (regular season) is the default. Run separately with
`-season-type 3` for postseason. **Season and season type select the dataset.**
The date flags only describe the ESPN calendar dates you expect to have imported;
they do not filter exported games by UTC start time. Use the full backfill range
for a season audit. A stored date proves a fetch succeeded, not that ESPN supplied
every game. Compare coverage against the season schedule before modeling.

Resolve findings by running the corresponding game/player imports and checking
again. A non-final canceled or postponed game can require investigation rather
than another fetch. Use player `-force` to collect corrections to already stored
box scores when needed; the quality command itself never changes records.

Export after reviewing the report:

```sh
mkdir -p exports
go run ./cmd/data -action export -league NBA -season 2026 \
  -from 2025-10-01 -to 2026-06-30 -out exports/nba-2026

go run ./cmd/data -action export -league NFL -season 2025 \
  -from 2025-09-01 -to 2026-02-28 -out exports/nfl-2025
```

Each export creates a **new** directory and refuses to overwrite an existing one:

- `games.csv`: one row per stored final game, ESPN game/team IDs, scope, UTC start
  time, scores and a player-import-complete flag.
- `players.csv`: one row per game/player/statistic category, with ESPN IDs, player
  name, historical team/position/jersey, DNP and nullable starter flags, and
  `stats_json`. Parse this JSON column to retain provider metric names and original
  values such as `17/32`; NFL players can appear in multiple categories. Only
  intact completed game imports contribute player rows.
- `report.json`: schema version, scope, timestamp, counts, blocking issues, warnings and limitations.
  Written last, its presence marks a completed export. A failed export cleans up
  its newly created directory. CSV rows are ordered by game time/ID, then player
  ID/category, and CSV quoting preserves commas and quotes in names and JSON.

Report schema version 2 separates blocking `issues` from non-blocking `warnings`.
NFL passing `adjQBR: "--"` is an unavailable optional derived rating: it produces
an `unavailable_adjusted_qbr` warning and remains unchanged in exported raw stats.
Missing core stats still block export, even when the same row has a QBR warning.
Metric-level findings include the game, player, category and metric key.
No migration or NFL reimport is needed to apply this reporting change.

By default, blocking quality issues prevent export. For an intentional partial dataset,
add `-allow-incomplete`; the report keeps its findings and
`ready_for_export: false`. Non-final games and partial player imports remain
excluded. Missing CSV values are empty, never converted to zero. The `exports/`
directory is ignored by Git. Both commands currently have a five-minute database
read deadline and support Ctrl+C.

These exports are **observations, not model features**: scores and player box
scores become available after the game. Next we will build chronological features
from earlier games and a separate target; do not use the current game's outcome
or box score as a predictor.

## Stop and restart

```sh
docker compose down
```

This stops the services and preserves database data in the named volume. Restart
with `docker compose up -d db`; the existing database does not need the league
migration reapplied.

## Troubleshooting

The ESPN client uses Go's standard `Go-http-client/1.1` User-Agent and requests
JSON. Local curl diagnostics returned 403 with the previous custom User-Agent
and 200 with the standard Go value, making that header a suspected cause rather
than proof of the restriction's source. After pulling this change, verify a single
team import before starting a full season. If a complete team import is already
stored, `-force` is needed to exercise the HTTP request. If it returns 403 again,
stop; the client still does not retry 403/429 responses. No browser cookies or
additional configuration are needed.

- **Missing configuration:** copy `.env.example` to `.env`, check the required
  settings, and run from the repository root.
- **Connection refused:** check `docker compose ps` and database readiness.
- **Port 5432 is already allocated:** stop the conflicting local database, or
  change `POSTGRES_PORT` in `.env` and update `DATABASE_URL` and
  `TEST_DATABASE_URL` there to match.
- **`relation "leagues" does not exist`:** apply the league migration to the
  database your command connects to.
- **Password authentication failed:** confirm the URL matches the database's
  credentials. An existing named volume retains its original database setup;
  editing `.env` credentials does not change an existing database password.

## Tests

With `.env` configured and PostgreSQL running:

```sh
go test -race -count=1 -v ./...
go vet ./...
```

Integration tests read `TEST_DATABASE_URL` from the repository-root `.env`, with
exported values taking precedence. To run only tests that need no database:

```sh
TEST_DATABASE_URL= go test ./...
```

An empty or absent `TEST_DATABASE_URL` skips integration tests. A configured but
unreachable database fails them. The test user needs permission to create schemas.
Each integration test creates its own schema, applies the required migrations,
and drops that schema on cleanup. Existing application tables are not changed,
and no manual migration is required for tests.

Additional tests cover ESPN response validation, request pacing, retry limits,
access restrictions, cancellation, ingestion failures, atomic team/game upserts,
NBA/NFL mapping, worker bounds, partial progress, stale-response protection,
database-first request skipping, forced refreshes, and atomic import records.
Coverage includes seed records and context forwarding, wrapped write failures and
stopping on error, repeated seeding without duplicates, updates preserving row
identity and creation time, timestamp refresh, and wrapped PostgreSQL errors.

## GitHub Actions

The `Go CI` workflow runs on pull requests, pushes to `main`, and manual runs
from the Actions tab. It has two checks:

- **Lint:** verifies `gofmt` formatting and runs golangci-lint v2.13.2 with
  `errcheck`, `govet`, `ineffassign`, `staticcheck`, and `unused`.
- **Tests and coverage:** starts PostgreSQL using the Compose configuration and `.env.example` and runs all tests,
  including integration tests, with race detection and coverage across all packages.

Both jobs use the Go version in `go.mod`. No repository secrets are needed;
CI copies `.env.example` to `.env`, starts a temporary Compose database, and
removes its volume afterward. Local credentials are never uploaded.

Open a workflow run in the Actions tab to see the coverage summary. Download its
`coverage` artifact for the raw profile, function summary, and browsable HTML
report. Artifacts are retained for 14 days. Coverage is reported without enforcing
a minimum percentage while the learning project's test suite grows.

To run the same linter locally:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
golangci-lint run
```

Ensure your Go binary directory (normally `$(go env GOPATH)/bin`) is on `PATH`.
To generate coverage locally, configure `TEST_DATABASE_URL` in `.env` and start
PostgreSQL as described above, then:

```sh
go test -race -count=1 -timeout=5m -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
```
