import csv
import json
import tempfile
import subprocess
import sys
import unittest
from dataclasses import replace
from datetime import datetime, timedelta, timezone
from pathlib import Path

import joblib
import numpy as np

from modeling.data import Game, load_export
from modeling.features import build_examples, split_examples
from modeling.train import fit_baseline, metrics, run


def games():
    start = datetime(2025, 9, 1, 18, tzinfo=timezone.utc)
    return [
        Game(str(i), start + timedelta(days=i), "a", "b", 20 + (i % 3) * 5, 24)
        for i in range(40)
    ]


def export_fixture(path, league="NFL"):
    data = games()
    report = {
        "schema_version": 3,
        "ready_for_export": True,
        "issues": [],
        "warnings": [],
        "missing_import_dates": [],
        "scope": {"league": league, "season": 2025, "season_type": 2},
        "final_games": len(data),
        "complete_player_games": len(data),
        "player_category_rows": len(data) * 2,
    }
    (path / "report.json").write_text(json.dumps(report))
    with (path / "games.csv").open("w", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            [
                "game_id",
                "league",
                "season",
                "season_type",
                "starts_at_utc",
                "home_team_id",
                "away_team_id",
                "home_score",
                "away_score",
                "player_import_complete",
            ]
        )
        for game in data:
            writer.writerow(
                [
                    game.id,
                    league,
                    2025,
                    2,
                    game.starts_at.isoformat(),
                    "a",
                    "b",
                    game.home_score,
                    game.away_score,
                    "true",
                ]
            )
    with (path / "players.csv").open("w", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            [
                "game_id",
                "player_id",
                "player_name",
                "team_id",
                "category",
                "did_not_play",
                "starter",
                "stats_json",
                "participation_status",
            ]
        )
        for game in data:
            for team in ("a", "b"):
                writer.writerow(
                    [
                        game.id,
                        team,
                        'Name, "quoted"',
                        team,
                        "passing",
                        "false",
                        "",
                        '{"adjQBR":"--"}',
                        "reported",
                    ]
                )


class FeaturesTest(unittest.TestCase):
    def test_target_and_future_changes_do_not_change_features(self):
        original, _ = build_examples(games())
        changed = [
            replace(g, home_score=999) if int(g.id) >= 15 else g for g in games()
        ]
        revised, _ = build_examples(changed)
        for a, b in zip(original, revised):
            if int(a.game_id) <= 15:
                self.assertEqual(a.values, b.values)
        self.assertNotEqual(original[-1].values, revised[-1].values)

    def test_same_day_excluded_and_unsorted_input(self):
        data = games()
        extra = replace(
            data[12],
            id="extra",
            starts_at=data[12].starts_at + timedelta(hours=1),
            home_score=800,
        )
        examples, _ = build_examples([*reversed(data), extra])
        by_id = {r.game_id: r for r in examples}
        self.assertEqual(by_id["12"].values, by_id["extra"].values)
        self.assertEqual(
            build_examples(data)[0], build_examples(list(reversed(data)))[0]
        )

    def test_single_class_evaluation(self):
        rows, _ = build_examples(games())
        result = metrics([replace(r, target=1) for r in rows], [0.5] * len(rows))
        self.assertAlmostEqual(result["brier_score"], 0.25)
        self.assertAlmostEqual(result["log_loss"], 0.69314718056)

    def test_dates_stay_together(self):
        data = games()
        data += [replace(g, id="extra" + g.id) for g in data]
        splits = split_examples(build_examples(data)[0])
        membership = {}
        for name, rows in splits.items():
            for row in rows:
                self.assertEqual(membership.setdefault(row.date, name), name)

    def test_known_feature_values(self):
        data = [replace(g, home_score=30, away_score=20) for g in games()]
        rows, _ = build_examples(data)
        self.assertEqual(rows[0].values, [1, 10, -10, 20, 1, 20, 0])

    def test_warmup_and_ties(self):
        data = games()
        data[8] = replace(data[8], home_score=24)
        examples, counts = build_examples(data)
        self.assertEqual(counts["excluded_warmup"], 5)
        self.assertEqual(counts["excluded_ties"], 1)
        self.assertEqual(len(examples), 34)
        self.assertNotIn("8", [r.game_id for r in examples])

    def test_splits_and_training_only_scaler(self):
        rows, _ = build_examples(games())
        splits = split_examples(rows)
        self.assertLess(splits["train"][-1].date, splits["validation"][0].date)
        self.assertLess(splits["validation"][-1].date, splits["test"][0].date)
        model, result, _ = fit_baseline(splits)
        np.testing.assert_allclose(
            model[0].mean_, np.mean([r.values for r in splits["train"]], axis=0)
        )
        # Changing test labels/features must not affect tuning or model coefficients.
        splits["test"] = [
            replace(r, target=1 - r.target, values=[1000.0] * 7) for r in splits["test"]
        ]
        revised, result2, _ = fit_baseline(splits)
        self.assertEqual(result["selected_C"], result2["selected_C"])
        np.testing.assert_array_equal(model[1].coef_, revised[1].coef_)

    def test_insufficient_history_and_single_training_class(self):
        with self.assertRaises(ValueError):
            split_examples(build_examples(games()[:7])[0])
        with self.assertRaises(ValueError):
            split_examples(
                build_examples([replace(g, home_score=50) for g in games()])[0]
            )


class ExportTest(unittest.TestCase):
    def test_run_both_leagues_and_reload(self):
        for league in ("NBA", "NFL"):
            with self.subTest(league=league), tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp)
                export_fixture(path, league)
                loaded, report, hashes = load_export(path)
                self.assertEqual(len(loaded), 40)
                self.assertEqual(len(hashes), 3)
                output = path / "run"
                manifest = run(path, output)
                self.assertEqual(manifest["scope"], report["scope"])
                self.assertEqual(manifest["input_sha256"], hashes)
                model = joblib.load(output / "model.joblib")
                rows = split_examples(build_examples(loaded)[0])["test"]
                with (output / "predictions.csv").open() as handle:
                    saved = [r for r in csv.DictReader(handle) if r["split"] == "test"]
                np.testing.assert_allclose(
                    model.predict_proba([r.values for r in rows])[:, 1],
                    [float(r["model_probability"]) for r in saved],
                )
                with self.assertRaises(ValueError):
                    run(path, output)

    def test_cli(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)
            export_fixture(path)
            command = [
                sys.executable,
                "-m",
                "modeling.train",
                "--input",
                str(path),
                "--out",
                str(path / "run"),
            ]
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["counts"]["eligible_games"], 35)
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 1)
            self.assertIn("output already exists", result.stderr)

    def test_reject_bad_exports(self):
        for case in (
            "schema",
            "issues",
            "count",
            "duplicate",
            "scope",
            "timestamp",
            "score",
            "reference",
            "status",
        ):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp)
                export_fixture(path)
                report = json.loads((path / "report.json").read_text())
                if case == "schema":
                    report["schema_version"] = 2
                if case == "issues":
                    report["issues"] = [{"code": "bad"}]
                if case == "count":
                    report["final_games"] = 999
                (path / "report.json").write_text(json.dumps(report))
                target = path / (
                    "players.csv" if case in ("reference", "status") else "games.csv"
                )
                with target.open() as handle:
                    rows = list(csv.DictReader(handle))
                headers = list(rows[0])
                if case == "duplicate":
                    rows[1]["game_id"] = rows[0]["game_id"]
                if case == "scope":
                    rows[0]["league"] = "NBA"
                if case == "timestamp":
                    rows[0]["starts_at_utc"] = "2025-09-01T18:00:00"
                if case == "score":
                    rows[0]["home_score"] = "--"
                if case == "reference":
                    rows[0]["game_id"] = "unknown"
                if case == "status":
                    rows[0]["participation_status"] = "did_not_play"
                with target.open("w", newline="") as handle:
                    writer = csv.DictWriter(handle, fieldnames=headers)
                    writer.writeheader()
                    writer.writerows(rows)
                with self.assertRaises(ValueError):
                    run(path, path / "run")
                self.assertFalse((path / "run").exists())


if __name__ == "__main__":
    unittest.main()
