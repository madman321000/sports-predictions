"""Validate a single league/season export without changing observations."""

import csv
import hashlib
import io
import json
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path


@dataclass(frozen=True)
class Game:
    id: str
    starts_at: datetime
    home: str
    away: str
    home_score: int
    away_score: int


def require(condition, message):
    if not condition:
        raise ValueError(message)


def csv_rows(body, required):
    reader = csv.DictReader(io.StringIO(body.decode("utf-8")))
    headers = reader.fieldnames or []
    require(
        len(headers) == len(set(headers)) and set(required) <= set(headers),
        "missing or duplicate CSV columns",
    )
    rows = list(reader)
    require(
        all(
            None not in row and all(v is not None for v in row.values()) for row in rows
        ),
        "malformed CSV row",
    )
    return rows


def load_export(directory: Path):
    # Parse the exact bytes hashed in the manifest, even if a file later changes.
    bodies = {
        name: (directory / name).read_bytes()
        for name in ("report.json", "games.csv", "players.csv")
    }
    hashes = {name: hashlib.sha256(body).hexdigest() for name, body in bodies.items()}
    report = json.loads(bodies["report.json"])
    require(isinstance(report, dict), "quality report must be an object")
    require(
        report.get("schema_version") == 3
        and report.get("ready_for_export") is True
        and report.get("issues") == []
        and report.get("missing_import_dates") == [],
        "a ready schema-v3 export without blocking issues is required",
    )
    scope = report["scope"]
    require(isinstance(scope, dict), "scope must be an object")
    require(
        scope["league"] in ("NBA", "NFL")
        and scope["season_type"] in (2, 3)
        and type(scope["season"]) is int
        and 1900 <= scope["season"] <= 2200,
        "invalid season scope",
    )
    games = []
    ids = {}
    for row in csv_rows(
        bodies["games.csv"],
        (
            "game_id",
            "league",
            "season",
            "season_type",
            "starts_at_utc",
            "home_team_id",
            "away_team_id",
            "home_score",
            "away_score",
            "player_import_complete",
        ),
    ):
        require(
            row["league"] == scope["league"]
            and row["season"] == str(scope["season"])
            and row["season_type"] == str(scope["season_type"]),
            "mixed game scope",
        )
        require(row["player_import_complete"] == "true", "incomplete player import")
        start = datetime.fromisoformat(row["starts_at_utc"].replace("Z", "+00:00"))
        require(start.tzinfo is not None, "game timestamp requires timezone")
        require(
            row["home_score"].isdigit() and row["away_score"].isdigit(), "invalid score"
        )
        game = Game(
            row["game_id"],
            start.astimezone(timezone.utc),
            row["home_team_id"],
            row["away_team_id"],
            int(row["home_score"]),
            int(row["away_score"]),
        )
        require(game.id and game.id not in ids, "empty or duplicate game ID")
        require(game.home and game.away and game.home != game.away, "invalid teams")
        require(
            scope["league"] != "NBA" or game.home_score != game.away_score,
            "NBA final game tied",
        )
        ids[game.id] = game
        games.append(game)
    require(
        games
        and len(games) == report["final_games"] == report["complete_player_games"],
        "game count differs from report",
    )
    seen, teams = set(), {g.id: set() for g in games}
    players = csv_rows(
        bodies["players.csv"],
        (
            "game_id",
            "player_id",
            "player_name",
            "team_id",
            "category",
            "did_not_play",
            "starter",
            "stats_json",
            "participation_status",
        ),
    )
    for row in players:
        game = ids.get(row["game_id"])
        require(game is not None, "player references unknown game")
        require(row["team_id"] in (game.home, game.away), "player team not in game")
        key = (game.id, row["player_id"], row["category"])
        require(
            all(key) and row["player_name"].strip() and key not in seen,
            "empty or duplicate player/category identity",
        )
        require(
            row["did_not_play"] in ("true", "false")
            and row["starter"] in ("", "true", "false"),
            "invalid player flags",
        )
        status = row["participation_status"]
        require(
            status in ("reported", "uncertain", "did_not_play")
            and (status == "did_not_play") == (row["did_not_play"] == "true"),
            "invalid participation status",
        )
        stats = json.loads(row["stats_json"])
        require(
            isinstance(stats, dict) and all(isinstance(v, str) for v in stats.values()),
            "invalid raw player statistics",
        )
        seen.add(key)
        teams[game.id].add(row["team_id"])
    require(
        len(players) == report["player_category_rows"],
        "player count differs from report",
    )
    require(
        all(len(value) == 2 for value in teams.values()),
        "game missing a team's player rows",
    )
    return sorted(games, key=lambda g: (g.starts_at, g.id)), report, hashes
