import json
import tempfile
import unittest
from pathlib import Path

from modeling.publish import public_report


class PublishTest(unittest.TestCase):
    def test_walk_forward_and_cross_season_strip_private_metadata(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            metrics = dict(games=1, log_loss=0.2, brier_score=0.1, accuracy=1)
            source = dict(
                scope=dict(league="NBA", season=2026),
                created_at="now",
                limitations=["backtest"],
                input_sha256="private",
                aggregate={
                    "strength": dict(
                        metrics=metrics, calibration=[], private_path="private"
                    )
                },
            )
            (root / "evaluation.json").write_text(json.dumps(source))
            (root / "predictions.csv").write_text(
                "feature_set,game_id,date,home_win,home_probability\nstrength,g1,2026-01-01,1,0.8\n"
            )
            report = public_report(root)
            self.assertEqual(report["kind"], "walk_forward")
            self.assertEqual(report["predictions"][0]["probability"], 0.8)
            self.assertNotIn("private", json.dumps(report))
            report.update(
                kind="cross_season", training_season=2025, private_path="private"
            )
            report["predictions"][0]["private_path"] = "private"
            (root / "dashboard.json").write_text(json.dumps(report))
            published = public_report(root)
            self.assertEqual(published["training_season"], 2025)
            self.assertNotIn("private", json.dumps(published))
