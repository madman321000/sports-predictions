# Import ESPN data

[Project overview](../../README.md) · [Setup](../setup/README.md) · [Ingestion](../ingestion/README.md) · [Quality and export](../data/README.md) · [Development](../development/README.md)

Run shell commands from the repository root, even when reading this guide.

Complete [local setup](../setup/README.md) first. Import in order: teams → games → players. For full seasons, set `INGEST_TIMEOUT=4h` in your existing `.env` and run one importer process at a time.

Jump to [teams](#1-import-teams), [games](#2-import-games), [players](#3-import-players),
[resume/refresh](#resume-or-refresh-imports), [concurrency](#concurrency-and-pacing),
or [request policy](#request-policy).

## 1. Import teams

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

## 2. Import games

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
missing-game findings in the quality report. 

Scheduled games have null scores, while a played score of zero is preserved.
Imports handle in-progress, final, postponed, canceled, suspended, and delayed
games. Unknown statuses and malformed records fail that date rather than silently
being skipped. No-game days are valid and count as completed dates.

Game upserts preserve identity and update start times, status, and scores. Each
date is committed atomically. A missing team fails the batch with an instruction
to import teams; no placeholder teams are created. Older fetched snapshots cannot
overwrite newer ones when workers finish out of order. Missing games are not
deleted, so refresh the relevant dates to collect later results or schedule changes.

## 3. Import players

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
name, and conflicting duplicate categories still fail the import. One narrow NBA
exception handles contradictory provider placeholders: no player ID, explicitly
not a starter, reason `COACH'S DECISION`, minutes `--`, and every
other statistic exactly `0`, `0-0`, or `0/0`. These are counted as excluded
nonparticipating entries even when ESPN sets didNotPlay to false. The `active`
flag is not used to infer participation: the provider varies it on otherwise
identical missing-time placeholders. Any recorded
minutes or nonzero/unrecognized statistic still fails validation. The excluded
DNP count describes the current run and is not persisted as a roster record.

Season totals cover only imported games; they are not independently fetched ESPN
season totals. Importing seven days of games produces seven days of player data,
not a full season. Coverage checks cannot discover dates absent from your database.
To backfill the example seasons, run one range per league before
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

## Resume or refresh imports

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

## Concurrency and pacing

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

## Request policy

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

## ESPN errors

The client uses Go's standard `Go-http-client/1.1` User-Agent. HTTP 403 or 429
stops the client; repeated restarts do not resolve an access restriction. Respect
any `Retry-After` delay and investigate access before retrying. No browser cookies
are required. To verify connectivity after resolving an error, run one team import;
use `-force` only if a cached import would otherwise skip the request.

Next: [check quality and export](../data/README.md).
