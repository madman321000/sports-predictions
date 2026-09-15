"""Fixed exploratory candidates; changes require a new protocol version."""

PROTOCOL = {
    "version": 1,
    "candidates": {
        "NBA": ["strength", "strength_recent"],
        "NFL": ["recent_form", "strength_recent"],
    },
    "C_grid": [0.01, 0.1, 1.0, 10.0],
    "primary_metric": "log_loss",
    "warmup_games": 5,
    "selection": "C selected on validation only; no automatic candidate selection",
    "calibration": "descriptive only; no fitted recalibration",
}


def weekly_windows(examples):
    """Two validation weeks, two test weeks; initial training at least four weeks."""
    if any(r.week is None for r in examples):
        raise ValueError(
            "NFL evaluation requires ESPN week metadata; re-export with schema v4 (no migration). Refresh games only if stored weeks are missing."
        )
    weeks = sorted({r.week for r in examples})
    if len(weeks) < 8:
        raise ValueError("NFL evaluation needs at least eight eligible weeks")
    if weeks != list(range(weeks[0], weeks[-1] + 1)):
        raise ValueError("eligible NFL weeks have gaps; inspect import coverage")
    for earlier, later in zip(weeks, weeks[1:]):
        if max(r.date for r in examples if r.week == earlier) >= min(
            r.date for r in examples if r.week == later
        ):
            raise ValueError(
                "NFL weeks overlap in UTC dates; cannot create chronological week boundaries"
            )
    # Anchor at the end so all test blocks have exactly two complete week groups.
    first_test = 6 + (len(weeks) - 6) % 2
    windows = []
    for start in range(first_test, len(weeks), 2):
        train_weeks = set(weeks[: start - 2])
        validation_weeks = set(weeks[start - 2 : start])
        test_weeks = set(weeks[start : start + 2])
        split = {
            "train": [r for r in examples if r.week in train_weeks],
            "validation": [r for r in examples if r.week in validation_weeks],
            "test": [r for r in examples if r.week in test_weeks],
        }
        if len({r.target for r in split["train"]}) != 2:
            raise ValueError("NFL training window requires both outcome classes")
        windows.append(split)
    return windows
