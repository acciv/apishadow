import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from apishadow.diff import analyze_files, compare, json_diff


def record(identifier, role, body, status=200, duration=10):
    return {
        "id": identifier,
        "role": role,
        "path": "/v1/users",
        "method": "GET",
        "status": status,
        "body": json.dumps(body),
        "headers": {"content-type": "application/json"},
        "duration_ms": duration,
    }


class DiffTest(unittest.TestCase):
    def test_json_diff_ignores_volatile_field(self):
        primary = {"id": 1, "timestamp": "a"}
        shadow = {"id": 1, "timestamp": "b"}
        self.assertEqual(json_diff(primary, shadow), [])

    def test_json_diff_finds_change(self):
        primary = {"items": [1, 2], "version": "1.0"}
        shadow = {"items": [1, 3], "version": "2.0"}
        diffs = json_diff(primary, shadow)
        paths = {d["path"] for d in diffs}
        self.assertIn("$.items[1]", paths)
        self.assertIn("$.version", paths)

    def test_status_mismatch_is_major(self):
        result = compare(record("a", "primary", {"ok": True}), record("a", "shadow", {"ok": True}, status=500))
        self.assertEqual(result["verdict"], "major_diff")
        self.assertGreaterEqual(result["score"], 50)

    def test_body_change_is_minor(self):
        result = compare(record("a", "primary", {"v": 1}), record("a", "shadow", {"v": 2}))
        self.assertEqual(result["verdict"], "minor_diff")

    def test_analyze_files(self):
        with tempfile.TemporaryDirectory() as directory:
            primary = Path(directory) / "primary.jsonl"
            shadow = Path(directory) / "shadow.jsonl"
            primary.write_text(
                "\n".join(json.dumps(r) for r in [record("a", "primary", {"v": 1}), record("b", "primary", {"v": 1})])
            )
            shadow.write_text(
                "\n".join(json.dumps(r) for r in [record("a", "shadow", {"v": 1}), record("b", "shadow", {"v": 2})])
            )
            report = analyze_files(primary, shadow)
            self.assertEqual(report["summary"]["compared"], 2)
            self.assertEqual(report["summary"]["match"], 1)
            self.assertEqual(report["summary"]["minor_diff"], 1)


if __name__ == "__main__":
    unittest.main()
