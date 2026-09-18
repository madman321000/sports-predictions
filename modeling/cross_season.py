"""Freeze candidates on an earlier season, then backtest a later season."""

import argparse
import hashlib
import json
import shutil
import tempfile
from dataclasses import replace
from datetime import datetime, timezone
from importlib.metadata import version
from pathlib import Path

import joblib
from sklearn.exceptions import ConvergenceWarning

from .data import load_export
from .evaluate import FEATURE_SETS, diagnostics
from .features import FEATURES, build_examples
from .protocol import PROTOCOL
from .train import fit_baseline


def earlier_split(rows, league):
    if league == "NFL" and any(r.week is None for r in rows):
        raise ValueError("NFL week metadata is required")
    groups = sorted({r.week if league == "NFL" else r.date for r in rows})
    if None in groups or len(groups) < 5:
        raise ValueError(
            "earlier season requires at least five eligible date/week groups"
        )
    boundary = groups[len(groups) * 4 // 5]
    training = [r for r in rows if (r.week if league == "NFL" else r.date) < boundary]
    validation = [
        r for r in rows if (r.week if league == "NFL" else r.date) >= boundary
    ]
    if max(r.date for r in training) >= min(r.date for r in validation):
        raise ValueError("earlier training and validation dates overlap")
    if len({r.target for r in training}) != 2:
        raise ValueError("training requires home wins and losses")
    return training, validation


def run(earlier, later, output):
    if output.exists() or output.is_symlink():
        raise ValueError("output exists; choose a new run directory")
    old, old_report, old_hashes = load_export(earlier)
    new, new_report, new_hashes = load_export(later)
    a, b = old_report["scope"], new_report["scope"]
    if (
        a["league"] != b["league"]
        or a["season_type"] != 2
        or b["season_type"] != 2
        or a["season"] >= b["season"]
    ):
        raise ValueError("use regular seasons from one league, earlier season first")
    if max(g.starts_at for g in old) >= min(g.starts_at for g in new):
        raise ValueError("season dates overlap")
    if a["league"] == "NFL" and any(g.week is None for g in old):
        raise ValueError("NFL earlier-season tuning needs exported week metadata")
    old_rows, old_counts = build_examples(old)
    new_rows, new_counts = build_examples(new)  # Deliberately reset season history.
    if not new_rows:
        raise ValueError("later season has no eligible games")
    training, validation = earlier_split(old_rows, a["league"])
    results, predictions, models = {}, [], {}
    game_lookup = {g.id: g for g in new}
    for name in PROTOCOL["candidates"][a["league"]]:
        indices = FEATURE_SETS[name]
        split = {
            key: [replace(r, values=[r.values[i] for i in indices]) for r in rows]
            for key, rows in {
                "train": training,
                "validation": validation,
                "test": new_rows,
            }.items()
        }
        model, result, raw = fit_baseline(split)
        models[name] = model
        test = [r for r in raw if r[0] == "test"]
        results[name] = {
            "metrics": result["evaluation"]["test"]["logistic_regression"],
            "selected_C": result["selected_C"],
            "validation_candidates": result["validation_candidates"],
            "features": [FEATURES[i] for i in indices],
            **diagnostics(new_rows, [r[5] for r in test]),
        }
        for row in test:
            g = game_lookup[row[1]]
            predictions.append(
                {
                    "candidate": name,
                    "game_id": row[1],
                    "date": row[2],
                    "home_win": row[3],
                    "probability": row[5],
                    "home_team_id": g.home,
                    "away_team_id": g.away,
                    "home_score": g.home_score,
                    "away_score": g.away_score,
                }
            )
        if "constant_home_rate" not in results:
            results["constant_home_rate"] = {
                "metrics": result["evaluation"]["test"]["constant_home_rate"],
                **diagnostics(new_rows, [r[4] for r in test]),
            }
            for row in test:
                g = game_lookup[row[1]]
                predictions.append(
                    {
                        "candidate": "constant_home_rate",
                        "game_id": row[1],
                        "date": row[2],
                        "home_win": row[3],
                        "probability": row[4],
                        "home_team_id": g.home,
                        "away_team_id": g.away,
                        "home_score": g.home_score,
                        "away_score": g.away_score,
                    }
                )
    report = {
        "schema_version": 1,
        "kind": "cross_season",
        "league": a["league"],
        "title": f"{a['league']} {a['season']} → {b['season']}",
        "training_season": a["season"],
        "test_season": b["season"],
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "protocol": PROTOCOL,
        "counts": {"earlier": old_counts, "later": new_counts},
        "splits": {
            name: {"games": len(rows), "from": rows[0].date, "to": rows[-1].date}
            for name, rows in {
                "training": training,
                "validation": validation,
                "test": new_rows,
            }.items()
        },
        "input_sha256": {"earlier": old_hashes, "later": new_hashes},
        "source_sha256": {
            p.name: hashlib.sha256(p.read_bytes()).hexdigest()
            for p in Path(__file__).parent.glob("*.py")
        },
        "versions": {
            name: version(name) for name in ("scikit-learn", "numpy", "joblib")
        },
        "limitations": [
            "Historical development backtest on previously inspected seasons; not a fresh holdout or live forecast.",
            "Training uses the earliest 80% of earlier-season groups; C is selected on the final 20%. No refit on validation or later-season data.",
            "Later-season history starts empty and updates using strictly earlier UTC dates, with five-game warmup.",
            "Provider corrections and missing completion timestamps limit point-in-time fidelity.",
            "Team labels are ESPN IDs. No upcoming-game inference or player features.",
        ],
        "candidates": results,
        "predictions": predictions,
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix=".cross-season-", dir=output.parent))
    try:
        for name, model in models.items():
            joblib.dump(model, stage / f"{name}.joblib")
        (stage / "dashboard.json").write_text(
            json.dumps(report, indent=2, allow_nan=False) + "\n"
        )
        stage.rename(output)
    finally:
        if stage.exists():
            shutil.rmtree(stage)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--earlier", type=Path, required=True)
    parser.add_argument("--later", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    try:
        report = run(args.earlier, args.later, args.out)
    except (ValueError, KeyError, TypeError, OSError, ConvergenceWarning) as error:
        parser.exit(1, f"cross-season: {error}\n")
    print(
        json.dumps(
            {
                "output": str(args.out),
                "metrics": {
                    name: r["metrics"] for name, r in report["candidates"].items()
                },
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
