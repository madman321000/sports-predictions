import json
import subprocess
import sys
import tempfile
import unittest
from dataclasses import replace
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest.mock import patch

from modeling.features import Example
from modeling.protocol import PROTOCOL, weekly_windows
from modeling.prepare import prepare
from modeling.data import load_export
from test_modeling import export_fixture


class ProtocolTest(unittest.TestCase):
    def rows(self):
        start = datetime(2025, 9, 1, tzinfo=timezone.utc)
        return [
            Example(
                str(i),
                (start + timedelta(days=i * 7)).date().isoformat(),
                [0.0] * 7,
                i % 2,
                i + 1,
            )
            for i in range(13)
        ]

    def test_whole_weeks_with_unequal_game_counts(self):
        rows = self.rows()
        rows += [replace(rows[9], game_id="extra")]
        seen = set()
        for split in weekly_windows(rows):
            groups = [
                {r.week for r in split[key]} for key in ("train", "validation", "test")
            ]
            self.assertLess(max(groups[0]), min(groups[1]))
            self.assertLess(max(groups[1]), min(groups[2]))
            self.assertEqual(len(groups[1]), 2)
            self.assertEqual(len(groups[2]), 2)
            self.assertFalse(seen & groups[2])
            seen |= groups[2]
        self.assertIn(13, seen)

    def test_invalid_week_metadata(self):
        rows = self.rows()
        for invalid in (
            rows[:7],
            [replace(rows[0], week=None), *rows[1:]],
            rows[:4] + rows[5:],
            [replace(rows[0], date=rows[1].date), *rows[1:]],
        ):
            with self.assertRaises(ValueError):
                weekly_windows(invalid)

    def test_candidates_are_fixed(self):
        self.assertEqual(
            PROTOCOL["candidates"],
            {
                "NBA": ["strength", "strength_recent"],
                "NFL": ["recent_form", "strength_recent"],
            },
        )

    def test_legacy_export_loads_but_has_no_week(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            export_fixture(root)
            report = json.loads((root / "report.json").read_text())
            report["schema_version"] = 3
            (root / "report.json").write_text(json.dumps(report))
            import csv

            with (root / "games.csv").open() as handle:
                rows = list(csv.DictReader(handle))
            for row in rows:
                del row["week"]
            with (root / "games.csv").open("w", newline="") as handle:
                writer = csv.DictWriter(handle, fieldnames=list(rows[0]))
                writer.writeheader()
                writer.writerows(rows)
            self.assertTrue(all(g.week is None for g in load_export(root)[0]))

    def test_inventory_rejects_holdout_before_observations(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "report.json").write_text(
                json.dumps(
                    {"scope": {"league": "NBA", "season": 2027, "season_type": 2}}
                )
            )
            with patch("modeling.prepare.load_export") as loader:
                with self.assertRaises(ValueError):
                    prepare([root], "NBA", 2027)
                loader.assert_not_called()

    def test_two_development_seasons(self):
        with tempfile.TemporaryDirectory() as tmp:
            inputs = []
            for season in (2024, 2025):
                root = Path(tmp) / str(season)
                root.mkdir()
                export_fixture(root)
                for name in ("report.json", "games.csv"):
                    p = root / name
                    p.write_text(p.read_text().replace("2025", str(season)))
                inputs.append(root)
            command = [
                sys.executable,
                "-m",
                "modeling.prepare",
                "--league",
                "NFL",
                "--holdout-season",
                "2026",
                "--out",
                str(Path(tmp) / "inventory.json"),
            ]
            for directory in inputs:
                command += ["--input", str(directory)]
            completed = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 1)
            result = prepare(inputs, "NFL", 2026)
            self.assertEqual([r["season"] for r in result["development"]], [2024, 2025])
            with self.assertRaises(ValueError):
                prepare([inputs[0], inputs[0]], "NFL", 2026)
