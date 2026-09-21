import json
import math
import tempfile
import unittest
from pathlib import Path

import joblib

from modeling.export_model import export_model
from modeling.train import run
from test_modeling import export_fixture


class ExportModelTest(unittest.TestCase):
    def test_portable_weights_match_sklearn(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            data = root / "data"
            data.mkdir()
            export_fixture(data)
            run(data, root / "trained")
            artifact = export_model(
                root / "trained", "", "nfl-test-v1", root / "nfl.json"
            )
            fitted = joblib.load(root / "trained" / "model.joblib")
            for row in ([0.1, 3, -2, 5, 0.2, 4, 1], [-0.2, -7, 2, -9, 0, -3, -2]):
                z = artifact["intercept"] + sum(
                    (x - m) / s * c
                    for x, m, s, c in zip(
                        row,
                        artifact["mean"],
                        artifact["scale"],
                        artifact["coefficients"],
                    )
                )
                self.assertAlmostEqual(
                    1 / (1 + math.exp(-z)), fitted.predict_proba([row])[0, 1], places=12
                )
            self.assertEqual(artifact["minimum_games"], 5)
            self.assertNotIn("predictions", json.loads((root / "nfl.json").read_text()))
            with self.assertRaises(ValueError):
                export_model(root / "trained", "", "nfl-test-v1", root / "nfl.json")
