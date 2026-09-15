import json
import subprocess
import sys
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

from modeling.evaluate import diagnostics, evaluate, run, walk_forward
from modeling.features import build_examples
from test_modeling import export_fixture, games


class EvaluationTest(unittest.TestCase):
    def test_disjoint_test_windows_and_boundaries(self):
        rows, _ = build_examples(games())
        rows += [replace(r, game_id="copy" + r.game_id) for r in rows]
        windows = walk_forward(rows)
        seen = set()
        for split in windows:
            self.assertLess(
                max(r.date for r in split["train"]),
                min(r.date for r in split["validation"]),
            )
            self.assertLess(
                max(r.date for r in split["validation"]),
                min(r.date for r in split["test"]),
            )
            dates = {r.date for r in split["test"]}
            self.assertFalse(seen & dates)
            seen |= dates
        self.assertEqual(max(seen), max(r.date for r in rows))
        with self.assertRaises(ValueError):
            walk_forward(rows[:5])
        with self.assertRaises(ValueError):
            walk_forward([replace(r, target=1) for r in rows])

    def test_calibration_edges_and_error_order(self):
        rows, _ = build_examples(games())
        rows = [replace(r, target=y) for r, y in zip(rows, [1, 0, 1, 0])]
        result = diagnostics(rows, [0, 1, 0.5, 0.2])
        self.assertEqual(sum(b["games"] for b in result["calibration"]), 4)
        self.assertEqual(result["calibration"][0]["observed_home_win_rate"], 1)
        self.assertEqual(result["calibration"][9]["games"], 1)
        self.assertIsNone(result["calibration"][3]["mean_probability"])
        self.assertEqual(len(result["confident_errors"]), 2)
        self.assertEqual(result["confident_errors"][0]["confidence"], 1)
        self.assertEqual(sum(d["games"] for d in result["by_date"]), 4)

    def test_last_test_changes_do_not_change_tuning(self):
        rows, _ = build_examples(games())
        results, records = evaluate(rows)
        boundary = min(r.date for r in walk_forward(rows)[-1]["test"])
        revised = [
            replace(r, target=1 - r.target, values=[999.0] * 7)
            if r.date >= boundary
            else r
            for r in rows
        ]
        changed, _ = evaluate(revised)
        for before, after in zip(results["windows"], changed["windows"]):
            for name in before["comparisons"]:
                self.assertEqual(
                    before["comparisons"][name]["selected_C"],
                    after["comparisons"][name]["selected_C"],
                )
                self.assertEqual(
                    before["comparisons"][name]["validation_candidates"],
                    after["comparisons"][name]["validation_candidates"],
                )
        for name, aggregate in results["aggregate"].items():
            selected = [r for r in records if r[1] == name]
            self.assertEqual(len(selected), aggregate["metrics"]["games"])
            self.assertEqual(len(selected), len({r[2] for r in selected}))
            self.assertEqual(
                {r[2] for r in selected}, {r[2] for r in records if r[1] == "all"}
            )

    def test_cli_both_leagues_and_no_overwrite(self):
        for league in ("NBA", "NFL"):
            with self.subTest(league=league), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                export_fixture(root, league)
                output = root / "evaluation"
                command = [
                    sys.executable,
                    "-m",
                    "modeling.evaluate",
                    "--input",
                    str(root),
                    "--out",
                    str(output),
                ]
                completed = subprocess.run(command, capture_output=True, text=True)
                self.assertEqual(completed.returncode, 0, completed.stderr)
                self.assertEqual(len(json.loads(completed.stdout)["aggregate"]), 7)
                manifest = json.loads((output / "evaluation.json").read_text())
                repeated = run(root, root / "second-evaluation")
                self.assertEqual(manifest["aggregate"], repeated["aggregate"])
                self.assertEqual(manifest["input_sha256"], repeated["input_sha256"])
                self.assertEqual(len(manifest["windows"]), 4)
                self.assertEqual(manifest["scope"]["league"], league)
                self.assertTrue((output / "summary.md").exists())
                before = (output / "evaluation.json").read_bytes()
                with self.assertRaises(ValueError):
                    run(root, output)
                self.assertEqual(before, (output / "evaluation.json").read_bytes())
