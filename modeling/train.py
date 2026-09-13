"""Run with python -m modeling.train --input exports/nba-2026 --out modeling/runs/nba-2026."""

import argparse
import csv
import hashlib
import json
import platform
import shutil
import tempfile
import warnings
from datetime import datetime, timezone
from importlib.metadata import version
from pathlib import Path

import joblib
from sklearn.exceptions import ConvergenceWarning
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score, brier_score_loss, log_loss
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

from .data import load_export
from .features import FEATURES, build_examples, split_examples


def metrics(rows, probabilities):
    targets = [r.target for r in rows]
    return {
        "games": len(rows),
        "log_loss": float(log_loss(targets, probabilities, labels=[0, 1])),
        "brier_score": float(brier_score_loss(targets, probabilities)),
        "accuracy": float(
            accuracy_score(targets, [int(p >= 0.5) for p in probabilities])
        ),
    }


def fit_baseline(splits):
    train = splits["train"]
    rate = sum(r.target for r in train) / len(train)
    candidates, fitted = [], []
    # Fixed grid, selected by validation log loss. Never fit on validation/test.
    for c in (0.01, 0.1, 1.0, 10.0):
        model = make_pipeline(
            StandardScaler(), LogisticRegression(C=c, max_iter=2000, random_state=0)
        )
        with warnings.catch_warnings():
            warnings.simplefilter("error", ConvergenceWarning)
            model.fit([r.values for r in train], [r.target for r in train])
        probabilities = model.predict_proba([r.values for r in splits["validation"]])[
            :, 1
        ]
        candidates.append({"C": c, **metrics(splits["validation"], probabilities)})
        fitted.append(model)
    selected = min(range(len(candidates)), key=lambda i: candidates[i]["log_loss"])
    model = fitted[selected]
    results, predictions = {}, []
    for name in ("validation", "test"):
        rows = splits[name]
        probabilities = model.predict_proba([r.values for r in rows])[:, 1]
        results[name] = {
            "constant_home_rate": metrics(rows, [rate] * len(rows)),
            "logistic_regression": metrics(rows, probabilities),
        }
        for row, probability in zip(rows, probabilities):
            predictions.append(
                [name, row.game_id, row.date, row.target, rate, float(probability)]
            )
    return (
        model,
        {
            "training_home_win_rate": rate,
            "selected_C": candidates[selected]["C"],
            "validation_candidates": candidates,
            "evaluation": results,
        },
        predictions,
    )


def run(directory, output):
    if output.exists() or output.is_symlink():
        raise ValueError("output already exists; choose a new run directory")
    games, report, hashes = load_export(directory)
    examples, counts = build_examples(games)
    splits = split_examples(examples)
    model, results, predictions = fit_baseline(splits)
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
            for name in ("data.py", "features.py", "train.py", "requirements.txt")
        },
        "random_state": 0,
        "feature_names": FEATURES,
        "history_policy": "strictly earlier UTC dates; minimum five games per team",
        "split_policy": "60/20/20 of eligible UTC dates; no refit after validation selection",
        "target": "home win; tied games excluded from targets, half-wins in historical form",
        "versions": {
            name: version(name) for name in ("scikit-learn", "numpy", "scipy", "joblib")
        },
        "python": platform.python_version(),
        "splits": {
            name: {
                "games": len(rows),
                "from": rows[0].date,
                "to": rows[-1].date,
                "home_wins": sum(r.target for r in rows),
            }
            for name, rows in splits.items()
        },
        "limitations": [
            "One season, particularly NFL, gives a small test sample.",
            "Historical export is not a point-in-time data archive; later provider corrections may remain.",
            "UTC date exclusion is conservative, not proof a prior game had finished.",
            "Player rows are validated but are not used as features.",
            "Earlier validation/test outcomes update later historical features; fitted parameters stay fixed.",
        ],
        **results,
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix=".model-stage-", dir=output.parent))
    try:
        joblib.dump(model, stage / "model.joblib")
        (stage / "manifest.json").write_text(
            json.dumps(manifest, indent=2, allow_nan=False) + "\n"
        )
        (stage / "quality-report.json").write_text(json.dumps(report, indent=2) + "\n")
        for name, headers, rows in (
            (
                "features.csv",
                ["split", "game_id", "date", "home_win", *FEATURES],
                [
                    [name, r.game_id, r.date, r.target, *r.values]
                    for name, rows in splits.items()
                    for r in rows
                ],
            ),
            (
                "predictions.csv",
                [
                    "split",
                    "game_id",
                    "date",
                    "home_win",
                    "constant_probability",
                    "model_probability",
                ],
                predictions,
            ),
        ):
            with (stage / name).open("w", newline="") as handle:
                writer = csv.writer(handle)
                writer.writerow(headers)
                writer.writerows(rows)
        stage.rename(output)
    finally:
        if stage.exists():
            shutil.rmtree(stage)
    return manifest


def main():
    parser = argparse.ArgumentParser(
        description="Train offline NBA or NFL home-win baselines"
    )
    parser.add_argument(
        "--input", required=True, type=Path, help="schema-v3 export directory"
    )
    parser.add_argument(
        "--out", required=True, type=Path, help="new model run directory"
    )
    args = parser.parse_args()
    try:
        result = run(args.input, args.out)
    except (ValueError, KeyError, TypeError, OSError, ConvergenceWarning) as error:
        parser.exit(1, f"modeling: {error}\n")
    print(
        json.dumps(
            {
                "output": str(args.out),
                "counts": result["counts"],
                "evaluation": result["evaluation"],
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
