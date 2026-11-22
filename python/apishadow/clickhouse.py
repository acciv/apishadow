"""Экспорт отчёта в NDJSON для загрузки в ClickHouse (см. docs/schema.sql)."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def to_rows(report: dict) -> list[dict]:
    rows = []
    for item in report.get("comparisons", []):
        status = item.get("status", {})
        rows.append(
            {
                "id": item.get("id"),
                "method": item.get("method"),
                "path": item.get("path"),
                "status_primary": status.get("primary"),
                "status_shadow": status.get("shadow"),
                "score": item.get("score"),
                "verdict": item.get("verdict"),
                "diff_json": json.dumps(
                    {
                        "header_diffs": item.get("header_diffs", []),
                        "body_diffs": item.get("body_diffs", []),
                        "timing": item.get("timing"),
                    },
                    ensure_ascii=False,
                ),
            }
        )
    return rows


def main() -> int:
    parser = argparse.ArgumentParser(description="Экспорт отчёта apishadow в NDJSON")
    parser.add_argument("--report", required=True)
    parser.add_argument("--out", default="diffs.ndjson")
    args = parser.parse_args()

    report = json.loads(Path(args.report).read_text(encoding="utf-8"))
    rows = to_rows(report)
    Path(args.out).write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows),
        encoding="utf-8",
    )
    print(f"rows={len(rows)} -> {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
