"""Expanding-window diagnostics; exploratory reuse of an observed season."""

import argparse
import csv
import hashlib
import json
import platform
import shutil
import tempfile
from collections import defaultdict
from dataclasses import replace
from datetime import datetime, timezone
from importlib.metadata import version
from pathlib import Path
from statistics import mean

from sklearn.exceptions import ConvergenceWarning

from .data import load_export
from .features import FEATURES, build_examples
from .train import fit_baseline, metrics

FEATURE_SETS = {
    "strength": [0, 1, 2, 3],
    "recent_form": [4, 5],
    "rest": [6],
    "strength_recent": [0, 1, 2, 3, 4, 5],
    "strength_rest": [0, 1, 2, 3, 6],
    "all": list(range(7)),
}


def walk_forward(examples):
    """Train/validation/test date shares: 40/20/10, 50/20/10, 60/20/10, 70/20/10."""
    dates = sorted({r.date for r in examples})
    if len(dates) < 10:
        raise ValueError(
            "at least ten eligible dates required for four evaluation windows"
        )
    windows = []
    for index in range(4):
        train_end = len(dates) * (4 + index) // 10
        test_start = len(dates) * (6 + index) // 10
        test_end = len(dates) * (7 + index) // 10
        split = {
            "train": [r for r in examples if r.date < dates[train_end]],
            "validation": [
                r for r in examples if dates[train_end] <= r.date < dates[test_start]
            ],
            "test": [
                r
                for r in examples
                if dates[test_start] <= r.date
                and (test_end == len(dates) or r.date < dates[test_end])
            ],
        }
        if len({r.target for r in split["train"]}) != 2:
            raise ValueError(
                f"window {index + 1}: training requires both outcome classes"
            )
        windows.append(split)
    return windows


def diagnostics(rows, probabilities):
    """Equal-width probability bins; empty bins are explicitly represented."""
    bins = [[] for _ in range(10)]
    by_date = defaultdict(list)
    wrong = []
    for row, p in zip(rows, probabilities, strict=True):
        p = float(p)
        if not 0 <= p <= 1:
            raise ValueError("invalid predicted probability")
        bins[min(int(p * 10), 9)].append((row.target, p))
        by_date[row.date].append((row, p))
        if int(p >= 0.5) != row.target:
            wrong.append(
                {
                    "game_id": row.game_id,
                    "date": row.date,
                    "home_win": row.target,
                    "home_probability": p,
                    "confidence": max(p, 1 - p),
                }
            )
    calibration = []
    for i, bucket in enumerate(bins):
        calibration.append(
            {
                "lower": i / 10,
                "upper": (i + 1) / 10,
                "games": len(bucket),
                "mean_probability": mean(p for _, p in bucket) if bucket else None,
                "observed_home_win_rate": mean(y for y, _ in bucket)
                if bucket
                else None,
            }
        )
    return {
        "calibration": calibration,
        "by_date": [
            {"date": date, **metrics([r for r, _ in values], [p for _, p in values])}
            for date, values in sorted(by_date.items())
        ],
        "confident_errors": sorted(
            wrong, key=lambda r: (-r["confidence"], r["date"], r["game_id"])
        )[:20],
    }


def evaluate(examples):
    windows = walk_forward(examples)
    evaluations, records = [], []
    pooled = defaultdict(list)
    for number, split in enumerate(windows, 1):
        window = {
            "window": number,
            "splits": {
                name: {
                    "games": len(rows),
                    "from": min(r.date for r in rows),
                    "to": max(r.date for r in rows),
                }
                for name, rows in split.items()
            },
            "comparisons": {},
        }
        for name, indices in FEATURE_SETS.items():
            subset = {
                key: [replace(r, values=[r.values[i] for i in indices]) for r in rows]
                for key, rows in split.items()
            }
            _, result, predictions = fit_baseline(subset)
            window["comparisons"][name] = result
            test_predictions = [p for p in predictions if p[0] == "test"]
            for row, prediction in zip(subset["test"], test_predictions, strict=True):
                probability = prediction[5]
                pooled[name].append((row, probability))
                records.append(
                    [number, name, row.game_id, row.date, row.target, probability]
                )
                if name == "all":
                    pooled["constant_home_rate"].append((row, prediction[4]))
                    records.append(
                        [
                            number,
                            "constant_home_rate",
                            row.game_id,
                            row.date,
                            row.target,
                            prediction[4],
                        ]
                    )
        evaluations.append(window)
    aggregate = {}
    for name, pairs in pooled.items():
        rows, probabilities = zip(*pairs)
        aggregate[name] = {
            "metrics": metrics(rows, probabilities),
            **diagnostics(rows, probabilities),
        }
    return {"windows": evaluations, "aggregate": aggregate}, records


def run(directory, output):
    if output.exists() or output.is_symlink():
        raise ValueError("output already exists; choose a new evaluation directory")
    games, report, hashes = load_export(directory)
    examples, counts = build_examples(games)
    results, records = evaluate(examples)
    manifest = {
        "schema_version": 1,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "scope": report["scope"],
        "input_sha256": hashes,
        "counts": counts,
        "source_sha256": {
            name: hashlib.sha256(
                Path(__file__).with_name(name).read_bytes()
            ).hexdigest()
            for name in (
                "evaluate.py",
                "train.py",
                "features.py",
                "data.py",
                "requirements.txt",
            )
        },
        "python": platform.python_version(),
        "versions": {
            name: version(name) for name in ("scikit-learn", "numpy", "scipy", "joblib")
        },
        "feature_sets": {
            name: [FEATURES[i] for i in indices]
            for name, indices in FEATURE_SETS.items()
        },
        "policy": "Four expanding training windows; next 20% of dates for validation; next 10% for test. Fixed C grid selected only on each validation window. No refit after selection.",
        "limitations": [
            "Exploratory diagnostics on an already observed season, not a fresh held-out performance claim.",
            "Do not select a feature set using these test results and claim unbiased test performance.",
            "Test windows are disjoint, but training overlaps and games are dependent; no independence assumption or significance claim.",
            "Later windows may train on earlier windows' test dates, as in sequential retraining.",
            "Small bins and date groups are noisy, especially NFL. Calibration is descriptive, not a fitted calibrator.",
            "Strictly earlier UTC-date history remains an approximation; exports lack completion times and point-in-time corrections.",
            "Ties excluded from targets; five-game warmup; no player features.",
        ],
        **results,
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix=".evaluation-stage-", dir=output.parent))
    try:
        (stage / "evaluation.json").write_text(
            json.dumps(manifest, indent=2, allow_nan=False) + "\n"
        )
        (stage / "quality-report.json").write_text(json.dumps(report, indent=2) + "\n")
        with (stage / "predictions.csv").open("w", newline="") as handle:
            writer = csv.writer(handle)
            writer.writerow(
                [
                    "window",
                    "feature_set",
                    "game_id",
                    "date",
                    "home_win",
                    "home_probability",
                ]
            )
            writer.writerows(records)
        summary = [
            "# Walk-forward evaluation",
            "",
            "Exploratory results on an observed season; these are not a fresh holdout.",
            "",
            "| Feature set | Games | Log loss | Brier score | Accuracy |",
            "| --- | ---: | ---: | ---: | ---: |",
        ]
        for name, result in manifest["aggregate"].items():
            m = result["metrics"]
            summary.append(
                f"| {name} | {m['games']} | {m['log_loss']:.4f} | {m['brier_score']:.4f} | {m['accuracy']:.1%} |"
            )
        summary += [
            "",
            "See evaluation.json for per-window metrics, calibration bins, date-level results and confidently wrong predictions.",
            "",
        ]
        (stage / "summary.md").write_text("\n".join(summary))
        stage.rename(output)
    finally:
        if stage.exists():
            shutil.rmtree(stage)
    return manifest


def main():
    parser = argparse.ArgumentParser(
        description="Explore walk-forward team-model diagnostics"
    )
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    try:
        result = run(args.input, args.out)
    except (ValueError, KeyError, TypeError, OSError, ConvergenceWarning) as error:
        parser.exit(1, f"evaluation: {error}\n")
    print(
        json.dumps(
            {
                "output": str(args.out),
                "counts": result["counts"],
                "aggregate": {
                    name: value["metrics"]
                    for name, value in result["aggregate"].items()
                },
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
