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
from .protocol import PROTOCOL, weekly_windows

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
        "calibration_gap": sum(
            len(b) * abs(mean(p for _, p in b) - mean(y for y, _ in b))
            for b in bins
            if b
        )
        / len(rows),
        "confident_predictions": {
            "threshold": 0.8,
            "games": sum(max(p, 1 - p) >= 0.8 for p in probabilities),
            "correct": sum(
                max(p, 1 - p) >= 0.8 and int(p >= 0.5) == r.target
                for r, p in zip(rows, probabilities)
            ),
        },
        "confident_errors": sorted(
            wrong, key=lambda r: (-r["confidence"], r["date"], r["game_id"])
        )[:20],
    }


def evaluate(examples, league=None):
    windows = weekly_windows(examples) if league == "NFL" else walk_forward(examples)
    candidates = PROTOCOL["candidates"][league] if league else list(FEATURE_SETS)
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
                    "weeks": sorted({r.week for r in rows if r.week is not None}),
                }
                for name, rows in split.items()
            },
            "comparisons": {},
        }
        for name in candidates:
            indices = FEATURE_SETS[name]
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
                if name == candidates[0]:
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
    if report["scope"]["league"] == "NFL" and (
        report["scope"]["season_type"] != 2 or any(g.week is None for g in games)
    ):
        raise ValueError(
            "NFL evaluation requires regular-season games with week metadata; re-export schema v4"
        )
    examples, counts = build_examples(games)
    results, records = evaluate(examples, report["scope"]["league"])
    manifest = {
        "schema_version": 2,
        "protocol": PROTOCOL,
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
                "protocol.py",
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
            if name in PROTOCOL["candidates"][report["scope"]["league"]]
        },
        "policy": "NFL: expanding training, two validation weeks and two test weeks; NBA: four expanding date windows. C selected on validation only, no refit. Candidate sets fixed by protocol.",
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
        for name, result in manifest["aggregate"].items():
            summary += [
                "",
                f"## {name}: calibration",
                "",
                f"Weighted absolute calibration gap: {result['calibration_gap']:.4f}",
                "",
                "| Probability bin | Games | Mean probability | Observed home-win rate |",
                "| --- | ---: | ---: | ---: |",
            ]
            for bucket in result["calibration"]:
                predicted = (
                    "—"
                    if bucket["mean_probability"] is None
                    else f"{bucket['mean_probability']:.3f}"
                )
                observed = (
                    "—"
                    if bucket["observed_home_win_rate"] is None
                    else f"{bucket['observed_home_win_rate']:.3f}"
                )
                summary.append(
                    f"| {bucket['lower']:.1f}–{bucket['upper']:.1f} | {bucket['games']} | {predicted} | {observed} |"
                )
            confident = result["confident_predictions"]
            summary += [
                "",
                f"At least 80% confidence: {confident['correct']} correct out of {confident['games']} predictions.",
            ]
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
