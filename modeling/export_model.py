"""Export a trusted local fitted model to a small JSON inference artifact."""

import argparse
import hashlib
import json
from pathlib import Path

import joblib
import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.preprocessing import StandardScaler

from .features import FEATURES


def export_model(run, candidate, model_id, output):
    if output.exists():
        raise ValueError("output exists; choose a versioned artifact path")
    if (run / "dashboard.json").exists():
        report = json.loads((run / "dashboard.json").read_text())
        if candidate not in report["candidates"] or candidate == "constant_home_rate":
            raise ValueError("select a fitted cross-season candidate")
        names = report["candidates"][candidate]["features"]
        league = report["league"]
        cutoff = report["splits"]["validation"]["to"]
        path = run / f"{candidate}.joblib"
    else:
        report = json.loads((run / "manifest.json").read_text())
        if report["scope"]["season_type"] != 2:
            raise ValueError("serving supports regular-season models only")
        names = report["feature_names"]
        league = report["scope"]["league"]
        cutoff = report["splits"]["validation"]["to"]
        path = run / "model.joblib"
    # Only use artifacts you trained locally: joblib/pickle is executable input.
    fitted = joblib.load(path)
    if len(fitted.steps) != 2:
        raise ValueError("expected scaler and logistic regression")
    scaler, classifier = fitted[0], fitted[1]
    if not isinstance(scaler, StandardScaler) or not isinstance(
        classifier, LogisticRegression
    ):
        raise ValueError("unsupported model pipeline")
    if list(classifier.classes_) != [0, 1] or classifier.coef_.shape != (1, len(names)):
        raise ValueError("expected binary home-win model")
    if not model_id.strip() or len(model_id) > 100 or league not in ("NBA", "NFL"):
        raise ValueError("invalid model ID or league")
    if len(set(names)) != len(names) or not set(names) <= set(FEATURES):
        raise ValueError("unsupported feature schema")
    artifact = {
        "schema_version": 1,
        "model_id": model_id,
        "league": league,
        "season_type": 2,
        "feature_policy": "strictly_prior_utc_dates_v1",
        "minimum_games": 5,
        "trained_through": cutoff,
        "features": names,
        "mean": scaler.mean_.tolist(),
        "scale": scaler.scale_.tolist(),
        "coefficients": classifier.coef_[0].tolist(),
        "intercept": float(classifier.intercept_[0]),
        "model_sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
    }
    if not np.all(np.array(artifact["scale"]) > 0):
        raise ValueError("invalid scaler")
    body = json.dumps(artifact, indent=2, allow_nan=False) + "\n"
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("x") as handle:
        handle.write(body)
    return artifact


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", type=Path, required=True)
    parser.add_argument("--candidate", default="")
    parser.add_argument("--model-id", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    try:
        artifact = export_model(args.run, args.candidate, args.model_id, args.out)
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        parser.exit(1, f"export model: {error}\n")
    print(
        json.dumps(
            {
                "output": str(args.out),
                "model_id": artifact["model_id"],
                "league": artifact["league"],
            }
        )
    )


if __name__ == "__main__":
    main()
