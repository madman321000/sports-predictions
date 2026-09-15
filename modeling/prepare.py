"""Inventory development seasons without loading a reserved holdout export."""

import argparse
import hashlib
import json
from pathlib import Path

from .data import load_export
from .features import build_examples
from .protocol import PROTOCOL


def prepare(inputs, league, holdout_season):
    if league not in PROTOCOL["candidates"] or not 1900 <= holdout_season <= 2200:
        raise ValueError("invalid league or holdout season")
    seasons, entries = set(), []
    for directory in inputs:
        # Reject a holdout before opening its games or player observations.
        report = json.loads((directory / "report.json").read_bytes())
        scope = report["scope"]
        if (
            scope["league"] != league
            or scope["season"] >= holdout_season
            or scope["season_type"] != 2
        ):
            raise ValueError(
                "development inputs must be regular seasons of the selected league before the reserved holdout"
            )
        if scope["season"] in seasons:
            raise ValueError("duplicate development season")
        games, loaded_report, hashes = load_export(directory)
        if loaded_report["scope"] != scope:
            raise ValueError(
                "export changed during inventory; retry with stable input files"
            )
        _, counts = build_examples(games)
        seasons.add(scope["season"])
        entries.append(
            {
                "season": scope["season"],
                "path": str(directory.resolve()),
                "input_sha256": hashes,
                "counts": counts,
            }
        )
    if len(seasons) < 2:
        raise ValueError("supply at least two distinct development seasons")
    return {
        "schema_version": 1,
        "league": league,
        "reserved_holdout_season": holdout_season,
        "protocol": PROTOCOL,
        "protocol_sha256": hashlib.sha256(
            Path(__file__).with_name("protocol.py").read_bytes()
        ).hexdigest(),
        "development": sorted(entries, key=lambda e: e["season"]),
        "status": "inventory only; no cross-season training or holdout evaluation performed",
        "policy": "Compute history within each season. Analyze development seasons separately. Lock candidate and tuning decisions before a later, never-inspected holdout is evaluated.",
    }


def main():
    parser = argparse.ArgumentParser(
        description="Prepare a development-season inventory and reserve a later holdout"
    )
    parser.add_argument("--input", action="append", required=True, type=Path)
    parser.add_argument("--league", choices=("NBA", "NFL"), required=True)
    parser.add_argument("--holdout-season", required=True, type=int)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    try:
        manifest = prepare(args.input, args.league, args.holdout_season)
        body = json.dumps(manifest, indent=2) + "\n"
        with args.out.open("x") as handle:
            handle.write(body)
    except (ValueError, KeyError, TypeError, OSError) as error:
        parser.exit(1, f"prepare: {error}\n")
    print(
        f"Saved development inventory to {args.out}; holdout observations were not loaded."
    )


if __name__ == "__main__":
    main()
