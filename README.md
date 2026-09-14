# sports-predictions

A learning project that imports NBA and NFL data from ESPN into PostgreSQL with
Go, then trains offline home-win prediction baselines in Python.

It supports teams, schedules, results and historical player box scores. Imports
resume from saved progress. The first models use historical team statistics;
player-level features are a later step.

## Start here

Follow the guides in order for a new installation, or jump to your current stage.
All commands run from the repository root.

| Stage | Guide |
| --- | --- |
| 1. Set up | [Prerequisites, `.env`, database, migrations and seeding](docs/setup/README.md) |
| 2. Import | [Teams, full seasons and player data](docs/ingestion/README.md) |
| 3. Export | [Quality checks, warnings and refreshing exports](docs/data/README.md) |
| 4. Model | [Train and evaluate separate NBA/NFL baselines](modeling/README.md) |

You need Go 1.27.1, Docker Compose and Git for ingestion. Modeling uses Python 3.12.
If your database and schema-v3 exports are already ready, go straight to the
[modeling guide](modeling/README.md#run-locally); no reimport is needed.

## Working on the project

- [`cmd/`](cmd/) contains the Go entrypoints for seeding, ingestion and export.
- [`internal/`](internal/) contains workflows, domain types, providers and repositories.
- [`migrations/`](migrations/) contains the PostgreSQL schema.
- [`modeling/`](modeling/) contains the Python baseline pipeline.

See [development and tests](docs/development/README.md) for architecture, lint,
integration tests, coverage and GitHub Actions.

## Keep in mind

Keep local credentials in `.env`; only `.env.example` belongs in Git. Apply database
migrations once, in order. Run one importer process at a time: workers share a
conservative request limiter, but separate processes do not.

Exports contain post-game observations. Predictive features must use earlier
games only. Player records describe box-score participation, not complete rosters,
and a successful quality check cannot prove ESPN supplied every scheduled game.
The models are learning baselines, with particularly limited NFL evaluation data.
