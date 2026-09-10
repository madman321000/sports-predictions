# sports-predictions

A Go learning project for a sports prediction platform. It seeds NBA/NFL league
reference data and imports NBA teams from ESPN into PostgreSQL. Game schedules,
results, and predictions are not implemented yet.

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

### 2. Start PostgreSQL

```sh
docker compose up -d db
docker compose exec -T db pg_isready -U sports -d sportsdb
```

Wait until the second command reports `accepting connections`. If the database
is still starting, rerun that command. Inspect startup problems with
`docker compose logs db`.

### 3. Configure the Go commands

```sh
export DATABASE_URL='postgres://sports:sports@localhost:5432/sportsdb?sslmode=disable'
```

Set this in each new terminal session before running the application. The current
code reads environment variables directly; it does not automatically load `.env`.

### 4. Apply the migrations

On a fresh database, run:

```sh
docker compose exec -T db psql -U sports -d sportsdb -v ON_ERROR_STOP=1 \
  < migrations/000001_create_leagues.up.sql
docker compose exec -T db psql -U sports -d sportsdb -v ON_ERROR_STOP=1 \
  < migrations/000002_create_teams.up.sql
```

This uses the PostgreSQL client inside the container; no local `psql` installation
is required. The project does not yet have a migration runner or migration history
table. Apply each migration once per database, in numeric order. If you already
applied the league migration, run only `000002_create_teams.up.sql`. If both
tables already exist, skip this step; these migrations are not designed to be rerun.

### 5. Seed NBA and NFL

```sh
go run ./cmd/seed
```

A successful run logs `Seeded Leagues`. Seeding can be rerun: existing leagues
are updated by abbreviation instead of duplicated.

Inspect the records:

```sh
docker compose exec -T db psql -U sports -d sportsdb \
  -c 'SELECT id, name, abbreviation, sport FROM leagues ORDER BY abbreviation;'
```

Expect two rows: NBA and NFL.

### 6. Import NBA teams from ESPN

```sh
go run ./cmd/ingest -league NBA
```

The command checks that NBA has been seeded, fetches the team list in one request,
and upserts the batch in a transaction. It logs the number imported and exits.
Reruns update teams by `(provider, league_id, external_id)` without duplicates;
missing teams are not deleted. Only NBA teams are supported in this first version.

To use a longer delay between requests:

```sh
go run ./cmd/ingest -league NBA -request-interval 10s
```

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
  Redirects are not followed. Requests have a 20-second timeout; the command has
  a five-minute deadline and supports cancellation with Ctrl+C.
- There is no automatic polling or per-team fan-out. Team reference data changes
  infrequently: run manually when needed, rather than on a frequent schedule.
- The limiter is per client/process and resets on restart. Run only one importer
  at a time. Multiple machines or processes would need a shared limiter before
  adding scheduled jobs or concurrency across processes.

Tests use a local HTTP server and a small synthetic ESPN-shaped fixture. CI never
calls the live ESPN endpoint.

## Stop and restart

```sh
docker compose down
```

This stops the services and preserves database data in the named volume. Restart
with `docker compose up -d db`; the existing database does not need the league
migration reapplied.

## Troubleshooting

- **`DATABASE_URL environment variable is not set`:** rerun the export above in
  the same terminal as the Go command.
- **Connection refused:** check `docker compose ps` and database readiness.
- **Port 5432 is already allocated:** stop the conflicting local database, or
  change the host port in Compose and update `DATABASE_URL` and
  `TEST_DATABASE_URL` to match.
- **`relation "leagues" does not exist`:** apply the league migration to the
  database your command connects to.
- **Password authentication failed:** confirm the URL matches the database's
  credentials. An existing named volume retains its original database setup;
  editing Compose credentials does not change an existing database password.

## Tests

Run unit tests and static checks:

```sh
go test ./...
go vet ./...
```

PostgreSQL integration tests skip when `TEST_DATABASE_URL` is unset. To run
all tests, including the real SQL upsert checks:

```sh
export TEST_DATABASE_URL='postgres://sports:sports@localhost:5432/sportsdb?sslmode=disable'
go test -race -count=1 -v ./...
```

Start the development database with `docker compose up -d db` if needed.
Use a local development or dedicated test database. The test user needs permission
to create schemas. Each integration test creates a uniquely named schema, applies
the required checked-in migrations there, and drops that schema on cleanup,
even when assertions fail. Existing application tables are not changed, and no
manual migration is required for these tests. A configured but unreachable database
fails the tests rather than skipping them.

Additional tests cover ESPN response validation, request pacing, retry limits,
access restrictions, cancellation, ingestion failures, and atomic team upserts.
Coverage includes seed records and context forwarding, wrapped write failures and
stopping on error, repeated seeding without duplicates, updates preserving row
identity and creation time, timestamp refresh, and wrapped PostgreSQL errors.

## GitHub Actions

The `Go CI` workflow runs on pull requests, pushes to `main`, and manual runs
from the Actions tab. It has two checks:

- **Lint:** verifies `gofmt` formatting and runs golangci-lint v2.13.2 with
  `errcheck`, `govet`, `ineffassign`, `staticcheck`, and `unused`.
- **Tests and coverage:** starts a fresh PostgreSQL 16 service and runs all tests,
  including integration tests, with race detection and coverage across all packages.

Both jobs use the Go version in `go.mod`. No repository secrets are needed;
PostgreSQL credentials belong only to the temporary CI database.

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
To generate coverage locally, set `TEST_DATABASE_URL` as described above, then:

```sh
go test -race -count=1 -timeout=5m -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
```
