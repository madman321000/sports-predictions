# Development seasons and a reserved holdout

[Modeling](../README.md) · [Evaluation](../evaluation/README.md) · [Ingestion](../../docs/ingestion/README.md)

NBA 2026 and NFL 2025 are already observed development seasons. Additional
windows within them do not create untouched test data. Import earlier seasons
for exploratory comparisons, and reserve a later season whose results you have
not used for model selection. The inventory below records that decision; it does
not train across seasons or open holdout observation files.

## Add an earlier development season

Run from the repository root. Keep one importer process running at a time, and
set `INGEST_TIMEOUT=4h` in your existing `.env`. Reuse seeded leagues and teams.
Example backfill ranges intentionally include off days:

```bash
go run ./cmd/ingest -resource games -league NBA -from 2024-10-01 -to 2025-06-30 -workers 4
go run ./cmd/ingest -resource players -league NBA -season 2025
go run ./cmd/data -action quality -league NBA -season 2025 -from 2024-10-01 -to 2025-06-30
go run ./cmd/data -action export -league NBA -season 2025 -from 2024-10-01 -to 2025-06-30 -out exports/nba-2025 -overwrite

go run ./cmd/ingest -resource games -league NFL -from 2024-09-01 -to 2025-02-28 -workers 4
go run ./cmd/ingest -resource players -league NFL -season 2024
go run ./cmd/data -action quality -league NFL -season 2024 -from 2024-09-01 -to 2025-02-28
go run ./cmd/data -action export -league NFL -season 2024 -from 2024-09-01 -to 2025-02-28 -out exports/nfl-2024 -overwrite
```

Resolve blocking quality findings before proceeding. Only explicitly verified, season-scoped
postponement mappings are reconciled; other postponed records need investigation. These are
regular-season exports; postseason is separate. Historical box scores describe
observed participation, not complete rosters.

## Record the development inputs and holdout choice

Activate the modeling environment. The following reserves NBA 2027 and NFL 2026;
use these only if they are genuinely uninspected for your modeling decisions.
You do not need their exports yet, and must not pass them as `--input`.

```bash
mkdir -p modeling/runs
python -m modeling.prepare --league NBA --holdout-season 2027 \
  --input exports/nba-2025 --input exports/nba-2026 \
  --out modeling/runs/nba-development-v1.json
python -m modeling.prepare --league NFL --holdout-season 2026 \
  --input exports/nfl-2024 --input exports/nfl-2025 \
  --out modeling/runs/nfl-development-v1.json
```

The command requires at least two distinct earlier regular seasons of one league.
It validates development exports, records their hashes/counts and the fixed
candidate protocol, and rejects later or duplicate inputs. Existing output files
are refused. A holdout input is rejected from its report before games or players
are read. This is a procedural reservation, not an access-control mechanism.

Evaluate development seasons separately with `modeling.evaluate` and distinct
output directories. History resets at each season; do not concatenate CSVs or let
end-of-season statistics initialize earlier games. Compare window and calibration
patterns across seasons, then freeze the candidate, tuning and calibration choices.
A future cross-season training/holdout command is separate work. Until that exists,
this inventory must not be presented as completed multi-season model validation.

When the reserved season is complete and quality-checked, evaluate the frozen
procedure once. Record all results; do not retune against that holdout and continue
calling it untouched. No holdout ingestion or evaluation occurs in this command.
