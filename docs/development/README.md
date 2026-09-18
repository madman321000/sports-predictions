# Development and tests

[Project overview](../../README.md) · [Setup](../setup/README.md) · [Ingestion](../ingestion/README.md) · [Quality and export](../data/README.md) · [Development](../development/README.md)

Run shell commands from the repository root, even when reading this guide.

## Code organization

- `modeling/`: offline Python export validation, historical features, training and evaluation.
- `internal/dataset/`: offline quality checks and CSV exports.
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

## Python modeling checks

See [modeling tests and CI](../../modeling/README.md#tests-and-ci) for Python setup,
lint, tests and coverage commands. The separate `Modeling CI` workflow runs those
checks on Python 3.12 and uploads a `modeling-coverage` artifact. Both workflows
use synthetic data and local test services; neither calls ESPN.

Statistics browsing uses `internal/stats` for HTTP validation and the repository
interface, and `internal/postgres/stats.go` for parameterized read queries. The
API opens a bounded read-only pool only when `STATS_DATABASE_URL` is configured.
Frontend browser tests run with `cd frontend && npm test`; set
`PLAYWRIGHT_PORT=5188` to avoid an already-running local dashboard.
