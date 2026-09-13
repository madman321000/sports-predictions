# Offline team baselines

This first model predicts home-win probability separately for NBA and NFL, using
schema-v3 exports from `cmd/data`. It never calls ESPN or connects to PostgreSQL.
No Go changes, database migrations, or player features are required.

## Run locally

From the repository root, use Python 3.12:

```bash
python3.12 -m venv modeling/.venv
source modeling/.venv/bin/activate
python -m pip install -r modeling/requirements.txt

python -m modeling.train --input exports/nba-2026 --out modeling/runs/nba-2026-v1
python -m modeling.train --input exports/nfl-2025 --out modeling/runs/nfl-2025-v1
```

Both input directories must contain `games.csv`, `players.csv` and `report.json`
from a successful export. The loader checks schema version, readiness, scope,
row counts, IDs, scores, timezone-aware timestamps, complete player imports,
player references and participation flags. Expected warnings do not block loading.
Raw QBR and uncertain-participation values are preserved; player statistics are
validated but are not used in this first model.

Use a new output directory for each experiment. Existing runs are never refreshed
by this command, so the previous evaluation remains available for comparison.
If input validation, training, or staging fails, no completed run is published.

## Features and time boundaries

Each row describes a single game. All features are home-team minus away-team:

| Feature | Calculation for each team |
| --- | --- |
| win_rate_diff | Win rate over all earlier games; a historical tie is half a win |
| points_for_diff | Mean points scored over all earlier games |
| points_against_diff | Mean points allowed over all earlier games |
| margin_diff | Mean points scored minus points allowed over all earlier games |
| last5_win_rate_diff | Win rate over the five most recent earlier games |
| last5_margin_diff | Mean scoring margin over those five games |
| rest_days_diff | UTC calendar days since the most recent earlier game |

A team needs five earlier games before its matchup is eligible. Ties are excluded
from binary prediction targets but remain in historical features. Warmup and tie
exclusions are counted separately, with ties taking precedence. NBA ties are
invalid input. These models target the league/season/type in the input report;
regular season and postseason cannot be mixed in one export.

Only strictly earlier UTC dates enter history. We build every row for a date
before adding any of that date's results. This excludes same-day results even
when the earlier game probably finished. Exports have start times, not completion
times, so this is a conservative approximation rather than verified as-of data.
Provider corrections made after a game can also remain in a historical export.

Eligible dates are divided approximately 60% training, 20% validation and 20%
test; all games on a date stay together. The minimum is five eligible dates and
both target classes in training, although meaningful evaluation needs much more.
Features for later validation/test games incorporate outcomes from earlier dates
in those periods, simulating sequential predictions. Model parameters stay fixed.

## Training and evaluation

The constant baseline predicts the training-set home-win rate for every game.
The second baseline uses a [scikit-learn pipeline](https://scikit-learn.org/stable/modules/generated/sklearn.pipeline.Pipeline.html)
with `StandardScaler` and [logistic regression](https://scikit-learn.org/stable/modules/generated/sklearn.linear_model.LogisticRegression.html).
The scaler and model fit only training data. Missing features are avoided by the
five-game history requirement; no full-dataset imputation is performed.

Choose `C` from the fixed grid `[0.01, 0.1, 1, 10]` using validation log loss.
Ties select the smaller `C`. The selected training-fitted model is then evaluated
on the test set without refitting or test-based selection. Convergence warnings
stop the run. Accuracy uses a fixed 0.5 threshold.

Both baselines report log loss, Brier score, accuracy and sample count for
validation and test. Lower log loss/Brier score is better; higher accuracy is
better. Inspect probability metrics first. No performance improvement is assumed.
One NFL season yields a small test sample. Treat results as an initial baseline,
not evidence of reliable forecasting. Repeatedly tuning after looking at the test
results invalidates its role as a held-out evaluation; reserve additional seasons
for later experiments.

## Saved artifacts

- `model.joblib`: fitted scaler and selected classifier, in feature-list order.
- `manifest.json`: source/input hashes, dependency versions, feature order,
  split dates and counts, exclusions, target/history policies, tuning and metrics.
- `features.csv`: feature rows, split membership and observed home-win targets.
- `predictions.csv`: validation/test probabilities for both baselines and outcomes.
- `quality-report.json`: input quality findings, including expected warnings.

Load only model files you trust: joblib uses Python pickle serialization. Generated
runs and virtual environments are ignored by Git. No real exports or results are
committed. The CLI prints counts and evaluation results after successful training;
those are the outputs to review before considering player features.

## Tests and CI

```bash
python -m ruff check modeling
python -m ruff format --check modeling
python -m coverage run --source=modeling --omit='modeling/tests/*' -m unittest discover -s modeling/tests -v
python -m coverage report -m
```

Tests cover malformed exports, counts/references, future/target/same-day leakage,
chronological splits, training-only scaling, test-independent model selection,
NFL ties, insufficient history, and end-to-end runs with model reload for both
leagues. Fixtures are synthetic; CI does not train against your local season data.
