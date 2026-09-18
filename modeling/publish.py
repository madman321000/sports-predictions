"""Create an explicitly public report from an existing evaluation or cross-season run."""

import argparse
import csv
import json
from pathlib import Path


def _read_report(directory):
    cross = directory / "dashboard.json"
    if cross.exists():
        source = json.loads(cross.read_text())
        return {
            key: source[key]
            for key in (
                "schema_version",
                "kind",
                "league",
                "title",
                "generated_at",
                "training_season",
                "test_season",
                "limitations",
                "candidates",
                "predictions",
            )
        }
    source = json.loads((directory / "evaluation.json").read_text())
    predictions = []
    with (directory / "predictions.csv").open() as handle:
        for row in csv.DictReader(handle):
            predictions.append(
                {
                    "candidate": row["feature_set"],
                    "game_id": row["game_id"],
                    "date": row["date"],
                    "home_win": int(row["home_win"]),
                    "probability": float(row["home_probability"]),
                }
            )
    scope = source["scope"]
    return {
        "schema_version": 1,
        "kind": "walk_forward",
        "league": scope["league"],
        "title": f"{scope['league']} {scope['season']} · walk-forward",
        "test_season": scope["season"],
        "generated_at": source["created_at"],
        "limitations": source["limitations"],
        "candidates": source["aggregate"],
        "predictions": predictions,
    }


def public_report(directory):
    report = _read_report(directory)
    report["candidates"] = {
        name: {
            "metrics": {
                key: item["metrics"][key]
                for key in ("games", "log_loss", "brier_score", "accuracy")
            },
            "calibration": [
                {
                    key: bucket[key]
                    for key in (
                        "lower",
                        "upper",
                        "games",
                        "mean_probability",
                        "observed_home_win_rate",
                    )
                }
                for bucket in item.get("calibration", [])
            ],
        }
        for name, item in report["candidates"].items()
    }
    fields = (
        "candidate",
        "game_id",
        "date",
        "home_win",
        "probability",
        "home_team_id",
        "away_team_id",
        "home_score",
        "away_score",
    )
    report["predictions"] = [
        {key: row[key] for key in fields if key in row} for row in report["predictions"]
    ]
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    try:
        body = json.dumps(public_report(args.run), indent=2, allow_nan=False) + "\n"
        with args.out.open("x") as handle:
            handle.write(body)
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, f"publish: {error}\n")
    print(f"Created public report {args.out}; inspect before uploading.")


if __name__ == "__main__":
    main()
