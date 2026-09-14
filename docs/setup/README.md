# Local setup

[Project overview](../../README.md) · [Setup](../setup/README.md) · [Ingestion](../ingestion/README.md) · [Quality and export](../data/README.md) · [Development](../development/README.md)

Run shell commands from the repository root, even when reading this guide.

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
table. Apply each migration once per database, in numeric order. Skip migrations already applied; these migrations are not designed to be rerun.

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

## Stop and restart

```sh
docker compose down
```

This stops the services and preserves database data in the named volume. Restart
with `docker compose up -d db`; the existing database does not need migrations reapplied.

## Troubleshooting

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

For ESPN errors and import recovery, see [ingestion](../ingestion/README.md#espn-errors).

Next: [import teams, games and players](../ingestion/README.md).
