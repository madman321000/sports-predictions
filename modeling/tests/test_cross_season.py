import json
import tempfile
import unittest
from pathlib import Path

import joblib
import numpy as np

from modeling.cross_season import run
from test_modeling import export_fixture


class CrossSeasonTest(unittest.TestCase):
    def test_frozen_models_and_reset_history(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            earlier, later = root / "earlier", root / "later"
            earlier.mkdir()
            later.mkdir()
            export_fixture(earlier)
            export_fixture(later)
            for name in ("report.json", "games.csv"):
                p = later / name
                p.write_text(p.read_text().replace("2025", "2026"))
            first = run(earlier, later, root / "first")
            self.assertEqual(first["counts"]["later"]["excluded_warmup"], 5)
            self.assertEqual(len(first["candidates"]), 3)
            # Reverse all later outcomes: no influence on earlier fits or C selection.
            import csv

            with (later / "games.csv").open() as f:
                rows = list(csv.DictReader(f))
            for r in rows:
                r["home_score"], r["away_score"] = r["away_score"], r["home_score"]
            with (later / "games.csv").open("w", newline="") as f:
                w = csv.DictWriter(f, fieldnames=list(rows[0]))
                w.writeheader()
                w.writerows(rows)
            second = run(earlier, later, root / "second")
            for name in ("recent_form", "strength_recent"):
                self.assertEqual(
                    first["candidates"][name]["selected_C"],
                    second["candidates"][name]["selected_C"],
                )
                a = joblib.load(root / "first" / f"{name}.joblib")
                b = joblib.load(root / "second" / f"{name}.joblib")
                np.testing.assert_array_equal(a[1].coef_, b[1].coef_)
                np.testing.assert_array_equal(a[0].mean_, b[0].mean_)
            self.assertEqual(
                json.loads((root / "first" / "dashboard.json").read_text())["league"],
                "NFL",
            )
            with self.assertRaises(ValueError):
                run(later, earlier, root / "bad")
            with self.assertRaises(ValueError):
                run(earlier, later, root / "first")
