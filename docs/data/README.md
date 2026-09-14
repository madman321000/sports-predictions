# Data quality and export

[Project overview](../../README.md) · [Setup](../setup/README.md) · [Ingestion](../ingestion/README.md) · [Quality and export](../data/README.md) · [Development](../development/README.md)

Run shell commands from the repository root, even when reading this guide.

## Check quality

After [setup](../setup/README.md) and [ingestion](../ingestion/README.md), use the
offline data command. It requires migrations through **000005** and stored games
and players. It makes no ESPN requests and reads `DATABASE_URL` from your existing
`.env`; ESPN settings are not required. Queries use a read-only, repeatable
database snapshot.

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
Use the same date range you imported. If your NBA backfill began on `2025-10-21`, use that date instead of `2025-10-01` in every command below.

The date flags only describe the ESPN calendar dates you expect to have imported;
they do not filter exported games by UTC start time. Use the full backfill range
for a season audit. A stored date proves a fetch succeeded, not that ESPN supplied
every game. Compare coverage against the season schedule before modeling.

Resolve findings by running the corresponding game/player imports and checking
again. A non-final canceled or postponed game can require investigation rather
than another fetch. Use player `-force` to collect corrections to already stored
box scores when needed; the quality command itself never changes records.

## Export or refresh

Export after reviewing the report:

```sh
mkdir -p exports
go run ./cmd/data -action export -league NBA -season 2026 \
  -from 2025-10-01 -to 2026-06-30 -out exports/nba-2026

go run ./cmd/data -action export -league NFL -season 2025 \
  -from 2025-09-01 -to 2026-02-28 -out exports/nfl-2025
```

Each export creates a new directory by default. Add `-overwrite` to refresh an
existing export for the same league, season and season type:

```bash
go run ./cmd/data -action export -league NFL -season 2025 \
  -from 2025-09-01 -to 2026-02-28 -out exports/nfl-2025 -overwrite

go run ./cmd/data -action export -league NBA -season 2026 \
  -from 2025-10-01 -to 2026-06-30 -out exports/nba-2026 -overwrite
```

Quality checks still apply. The command stages all files before replacing the
previous export; validation or staging failures leave it intact. Only directories
containing exactly the three regular export files and a valid report for the
same season may be replaced. Symlinks and unrelated files are rejected. Concurrent
exports to the same destination are rejected using a sibling `.lock` file.
Replacement uses a temporary backup and restores it if publishing fails. A process
or machine crash during replacement may leave a `.export-backup-*` directory and
lock file beside the destination; inspect and restore that backup before removing
the stale lock and retrying. This is not a filesystem transaction across crashes.

## Export files

The directory contains:

- `games.csv`: one row per stored final game, ESPN game/team IDs, scope, UTC start
  time, scores and a player-import-complete flag.
- `players.csv`: one row per game/player/statistic category, with ESPN IDs, player
  name, historical team/position/jersey, DNP and nullable starter flags, `participation_status`, and
  `stats_json`. Parse this JSON column to retain provider metric names and original
  values such as `17/32`; NFL players can appear in multiple categories. Only
  intact completed game imports contribute player rows.
- `report.json`: schema version, scope, timestamp, counts, blocking issues, warnings and limitations.
  Written last, its presence marks a completed export. A failed export cleans up
  its newly created directory. CSV rows are ordered by game time/ID, then player
  ID/category, and CSV quoting preserves commas and quotes in names and JSON.

## Warnings and incomplete data

Report schema version 3 retains the separation of blocking `issues` from non-blocking `warnings`.
NFL passing `adjQBR: "--"` is an unavailable optional derived rating: it produces
an `unavailable_adjusted_qbr` warning and remains unchanged in exported raw stats.
Missing core stats still block export, even when the same row has a QBR warning.
Metric-level findings include the game, player, category and metric key.

By default, blocking quality issues prevent export. For an intentional partial dataset,
add `-allow-incomplete`; the report keeps its findings and
`ready_for_export: false`. Non-final games and partial player imports remain
excluded. Missing CSV values are empty, never converted to zero. The `exports/`
directory is ignored by Git. Both commands currently have a five-minute database
read deadline and support Ctrl+C.

These exports are **observations, not model features**: scores and player box
scores become available after the game. The [modeling pipeline](../../modeling/README.md) builds chronological team features from earlier games; do not use the current game's outcome or box score as a predictor.

## NBA reconciliation

The quality command recognizes four explicitly reconciled ESPN replacements in
the NBA 2026 regular season:

| Postponed game | Replacement final game |
| --- | --- |
| 401810384 | 401850920 |
| 401810499 | 401857824 |
| 401810506 | 401858693 |
| 401810507 | 401858694 |

These mappings use the NBA's [Heat–Bulls announcement](https://pr.nba.com/heat-bulls-schedule-adjustments/),
[Warriors–Timberwolves announcement](https://www.nba.com/news/warriors-timberwolves-game-postponed),
and [weather rescheduling announcement](https://www.nba.com/news/nba-schedule-adjustments-weather-2026)
together with the stored ESPN IDs. The replacement must be present in the same
season snapshot, have the same home and away teams, occur later, be final with
scores, and have a complete player import. Only then does the postponed record
produce a `resolved_postponement` warning. Other unresolved games still block
export. The original records are preserved; only final games enter `games.csv`.

NBA general-stat rows with explicitly false `starter`, unavailable (`--`) minutes,
and the complete expected set of otherwise zero statistics produce an
`uncertain_participation` warning. Other missing or nonzero statistics retain the
existing blocking checks. This does not infer DNP or replace missing minutes with
zero. Schema version 3 appends `participation_status` to `players.csv`, with values
`reported`, `did_not_play`, or `uncertain`. `reported` means no special classification,
not independent verification of playing time. Exclude uncertain rows when counting
appearances or calculating per-appearance averages. Raw database season-total
`games_with_metric` counts remain counts of recorded metrics, not verified
appearances; use the exported participation flag for modeling.

Next: [train the offline models](../../modeling/README.md).
