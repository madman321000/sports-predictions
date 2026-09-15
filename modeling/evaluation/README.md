# Evaluation diagnostics

[Modeling overview](../README.md) · [Project overview](../../README.md)

Run from the repository root with the modeling virtual environment activated.
Use your schema-v4 exports for NFL; no migration is required. Re-export the stored data
to include ESPN week numbers:

```bash
go run ./cmd/data -action export -league NFL -season 2025 \
  -from 2025-09-01 -to 2026-02-28 -out exports/nfl-2025 -overwrite
```

NBA v3 exports still work. Missing stored weeks require inspecting and refreshing
the affected games; weeks are never guessed. Use new evaluation output names:

```bash
python -m modeling.evaluate --input exports/nba-2026 --out modeling/runs/nba-2026-eval-v2
python -m modeling.evaluate --input exports/nfl-2025 --out modeling/runs/nfl-2025-eval-v2
```

The evaluator writes a new directory and refuses existing paths. The original
training command and v1 model runs remain unchanged. This command produces
analysis, not a new deployment model or an automatic winning feature selection.

## Windows and comparisons

NBA retains four expanding windows of eligible UTC dates:

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

NFL regular-season evaluation uses stored ESPN `week`. Each window has at least
four training week groups, the next two for validation and the next two for test.
Training expands by two weeks; blocks are anchored at the end to avoid a final
one-week test block. Missing or nonconsecutive weeks and overlapping week dates
fail validation. At least eight eligible weeks are required. Complete groups do
not prove every scheduled game is stored; byes and warmup still affect counts.

Candidate protocol v1 is fixed in `modeling/protocol.py`:

| League | Reference | Challenger |
| --- | --- | --- |
| NBA | Strength | Strength + recent form |
| NFL | Recent form | Strength + recent form |

Both are compared with the training home-win rate. These choices were informed
by prior exploratory results. No candidate is automatically selected by test
performance; changing candidate definitions requires a new protocol version.

Each variant fits its own scaler and classifier on training data only and selects
C from the original fixed grid using validation log loss. No refitting follows
selection. Feature generation retains strictly earlier UTC-date history and the
five-game warmup. Earlier outcomes update later historical features within each
window; this simulates sequential predictions, not predictions made all at once.

## What to inspect

- `summary.md`: pooled metrics, calibration tables and correct/total counts at at least 80% confidence.
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

The weighted absolute calibration gap averages each bin's absolute difference
between predicted and observed home-win rates, weighted by sample count. This is
sensitive to binning and small samples; it is not a significance test or fitted
probability correction.

Next: [prepare development seasons and reserve a holdout](../seasons/README.md).
