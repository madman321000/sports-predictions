# Evaluation diagnostics

[Modeling overview](../README.md) · [Project overview](../../README.md)

Run from the repository root with the modeling virtual environment activated.
Use your existing schema-v3 exports; no migration or reimport is required.

```bash
python -m modeling.evaluate --input exports/nba-2026 --out modeling/runs/nba-2026-eval-v1
python -m modeling.evaluate --input exports/nfl-2025 --out modeling/runs/nfl-2025-eval-v1
```

The evaluator writes a new directory and refuses existing paths. The original
training command and v1 model runs remain unchanged. This command produces
analysis, not a new deployment model or an automatic winning feature selection.

## Windows and comparisons

The evaluator partitions eligible UTC dates into four expanding windows:

| Window | Training dates | Validation dates | Test dates |
| --- | --- | --- | --- |
| 1 | First 40% | Next 20% | Next 10% |
| 2 | First 50% | Next 20% | Next 10% |
| 3 | First 60% | Next 20% | Next 10% |
| 4 | First 70% | Next 20% | Final 10% |

Boundaries round down to whole dates. Every game on a date stays together.
At least ten eligible dates and both outcomes in each training window are required;
small samples can run but should not be interpreted as reliable estimates.
Test windows are disjoint. Later training windows can include earlier test results,
as would happen when retraining over time. There is no random shuffling.

Every window compares the same test games using:

- **Constant home rate:** calculated on that window's training games.
- **Strength:** season-to-date win rate, scoring, points allowed and margin.
- **Recent form:** last-five win rate and margin.
- **Rest:** days since the previous game.
- **Strength + recent form**, **strength + rest**, and **all features**.

Each variant fits its own scaler and classifier on training data only and selects
C from the original fixed grid using validation log loss. No refitting follows
selection. Feature generation retains strictly earlier UTC-date history and the
five-game warmup. Earlier outcomes update later historical features within each
window; this simulates sequential predictions, not predictions made all at once.

## What to inspect

- `summary.md`: pooled test metrics for every feature set and the constant baseline.
- `evaluation.json`: per-window results, selected C values, validation candidates,
  calibration bins, date-level metrics and the 20 most confidently wrong predictions
  per variant. Also includes scope, exclusion counts, versions and source/input hashes.
- `predictions.csv`: one test prediction per game and variant, with window identity.
- `quality-report.json`: the source export's quality report.

Pooled metrics use all test predictions, not an unweighted average of window
metrics. Compare each window too: improvement concentrated in one period is less
convincing than consistent improvement.

Calibration uses ten equal-width home-win probability bins. Each includes count,
mean predicted probability and observed home-win rate; empty bins contain nulls.
The final bin includes 1.0. This is descriptive calibration, not a fitted calibrator.
Per-date groups and NFL bins can be very small. Confident errors use the predicted
winner's probability, with a fixed home-win threshold of 0.5; game IDs identify
records for further inspection in the original exports.

These seasons have already been inspected. The additional windows are exploratory
reuse of known data, **not a fresh holdout**. Do not pick the best feature set here
and report its test score as an unbiased estimate. Preserve an additional season
for final evaluation. Overlapping training data and repeated teams also mean games
and window results are not independent; this report makes no significance claims.
