"""Only games on strictly earlier UTC dates may contribute to a feature row."""

from collections import defaultdict
from dataclasses import dataclass
from itertools import groupby
from statistics import mean

FEATURES = [
    "win_rate_diff",
    "points_for_diff",
    "points_against_diff",
    "margin_diff",
    "last5_win_rate_diff",
    "last5_margin_diff",
    "rest_days_diff",
]


@dataclass(frozen=True)
class Example:
    game_id: str
    date: str
    values: list[float]
    target: int
    week: int | None = None


def team_features(history, date):
    # history entries: date, points for, points against. Ties count as half wins
    # in historical form, but are excluded as binary prediction targets.
    def win(row):
        return 1.0 if row[1] > row[2] else 0.5 if row[1] == row[2] else 0.0

    return [
        mean(win(r) for r in history),
        mean(r[1] for r in history),
        mean(r[2] for r in history),
        mean(r[1] - r[2] for r in history),
        mean(win(r) for r in history[-5:]),
        mean(r[1] - r[2] for r in history[-5:]),
        float((date - history[-1][0]).days),
    ]


def build_examples(games):
    history = defaultdict(list)
    examples = []
    counts = {"input_games": len(games), "excluded_ties": 0, "excluded_warmup": 0}
    ordered = sorted(games, key=lambda g: (g.starts_at, g.id))
    for date, group in groupby(ordered, key=lambda g: g.starts_at.date()):
        batch = list(group)
        for game in batch:
            if game.home_score == game.away_score:
                counts["excluded_ties"] += 1
                continue
            if min(len(history[game.home]), len(history[game.away])) < 5:
                counts["excluded_warmup"] += 1
                continue
            home = team_features(history[game.home], date)
            away = team_features(history[game.away], date)
            examples.append(
                Example(
                    game.id,
                    date.isoformat(),
                    [h - a for h, a in zip(home, away)],
                    int(game.home_score > game.away_score),
                    game.week,
                )
            )
        # Updating after the entire date prevents same-day results leaking.
        for game in batch:
            history[game.home].append((date, game.home_score, game.away_score))
            history[game.away].append((date, game.away_score, game.home_score))
    counts["eligible_games"] = len(examples)
    return examples, counts


def split_examples(examples):
    dates = sorted({row.date for row in examples})
    if len(dates) < 5:
        raise ValueError("at least five eligible dates required for 60/20/20 splitting")
    first, second = int(len(dates) * 0.6), int(len(dates) * 0.8)
    splits = {"train": [], "validation": [], "test": []}
    for row in sorted(examples, key=lambda r: (r.date, r.game_id)):
        key = (
            "train"
            if row.date < dates[first]
            else "validation"
            if row.date < dates[second]
            else "test"
        )
        splits[key].append(row)
    if len({r.target for r in splits["train"]}) != 2:
        raise ValueError("training split must contain both home wins and losses")
    return splits
