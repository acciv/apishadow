"""Diff-анализ записей, которые записал Go-прокси через Rust-ядро."""

from __future__ import annotations

import argparse
import fnmatch
import json
import sys
from pathlib import Path
from typing import Any

VERSION = "0.1.0"

DEFAULT_IGNORE = (
    "timestamp",
    "request_id",
    "trace_id",
    "generated_at",
    "date",
    "server",
    "x-request-id",
    "x-trace-id",
    "x-correlation-id",
    "set-cookie",
    "content-length",
)


def _ignored(path: str, patterns: tuple[str, ...] | list[str]) -> bool:
    tail = path.rsplit(".", 1)[-1].lower()
    lowered = path.lower()
    return any(fnmatch.fnmatch(tail, p.lower()) or fnmatch.fnmatch(lowered, p.lower()) for p in patterns)


def _short(value: Any, limit: int = 200) -> Any:
    if isinstance(value, str) and len(value) > limit:
        return value[:limit] + "..."
    if isinstance(value, (list, dict)):
        text = json.dumps(value, ensure_ascii=False)
        if len(text) > limit:
            return text[:limit] + "..."
    return value


def json_diff(
    primary: Any,
    shadow: Any,
    path: str = "$",
    ignore: tuple[str, ...] | list[str] = DEFAULT_IGNORE,
) -> list[dict[str, Any]]:
    """Рекурсивно находит различия в двух JSON-значениях."""
    differences: list[dict[str, Any]] = []

    if isinstance(primary, dict) and isinstance(shadow, dict):
        for key in sorted(set(primary) | set(shadow)):
            child = f"{path}.{key}"
            if _ignored(child, ignore):
                continue
            if key not in primary:
                differences.append({"path": child, "kind": "added_in_shadow", "value": _short(shadow[key])})
                continue
            if key not in shadow:
                differences.append({"path": child, "kind": "removed_in_shadow", "value": _short(primary[key])})
                continue
            differences.extend(json_diff(primary[key], shadow[key], child, ignore))
        return differences

    if isinstance(primary, list) and isinstance(shadow, list):
        if len(primary) != len(shadow):
            differences.append(
                {"path": path, "kind": "length", "primary": len(primary), "shadow": len(shadow)}
            )
        for index in range(min(len(primary), len(shadow))):
            differences.extend(json_diff(primary[index], shadow[index], f"{path}[{index}]", ignore))
        return differences

    if primary != shadow:
        differences.append(
            {"path": path, "kind": "value", "primary": _short(primary), "shadow": _short(shadow)}
        )
    return differences


def _parse_body(body: str) -> tuple[bool, Any]:
    try:
        return True, json.loads(body) if body else None
    except (TypeError, ValueError):
        return False, body


def compare(
    primary: dict[str, Any],
    shadow: dict[str, Any],
    ignore: tuple[str, ...] | list[str] = DEFAULT_IGNORE,
) -> dict[str, Any]:
    """Сравнивает две нормализованные записи с одинаковым id."""
    status_primary = primary.get("status")
    status_shadow = shadow.get("status")
    status_diff = status_primary != status_shadow

    headers_primary = primary.get("headers") or {}
    headers_shadow = shadow.get("headers") or {}
    header_diffs: list[dict[str, Any]] = []
    for name in sorted(set(headers_primary) | set(headers_shadow)):
        if _ignored(name, ignore):
            continue
        left = headers_primary.get(name)
        right = headers_shadow.get(name)
        if left != right:
            header_diffs.append({"name": name, "primary": _short(left), "shadow": _short(right)})

    body_primary = primary.get("body", "")
    body_shadow = shadow.get("body", "")
    ok_primary, value_primary = _parse_body(body_primary)
    ok_shadow, value_shadow = _parse_body(body_shadow)
    if ok_primary and ok_shadow:
        body_diffs = json_diff(value_primary, value_shadow, "$", ignore)
    elif body_primary != body_shadow:
        body_diffs = [
            {
                "path": "$",
                "kind": "raw_body",
                "primary": _short(body_primary),
                "shadow": _short(body_shadow),
            }
        ]
    else:
        body_diffs = []

    timing: dict[str, Any] | None = None
    try:
        first = float(primary.get("duration_ms", 0))
        second = float(shadow.get("duration_ms", 0))
        if first > 0 and second > 0:
            ratio = max(first / second, second / first)
            if ratio >= 2:
                timing = {
                    "primary_ms": round(first, 2),
                    "shadow_ms": round(second, 2),
                    "ratio": round(ratio, 2),
                }
    except (TypeError, ValueError):
        pass

    score = 0
    if status_diff:
        score += 55
    score += min(15, 4 * len(header_diffs))
    score += min(35, 12 * len(body_diffs))
    if timing:
        score += 5
    score = min(100, score)

    if score >= 50:
        verdict = "major_diff"
    elif score >= 10:
        verdict = "minor_diff"
    else:
        verdict = "match"

    return {
        "id": primary.get("id") or shadow.get("id"),
        "path": primary.get("path") or shadow.get("path"),
        "method": primary.get("method") or shadow.get("method"),
        "status": {"primary": status_primary, "shadow": status_shadow, "different": status_diff},
        "header_diffs": header_diffs,
        "body_diffs": body_diffs,
        "timing": timing,
        "score": score,
        "verdict": verdict,
    }


def compare_record(
    primary: dict[str, Any],
    shadow: dict[str, Any],
    ignore: tuple[str, ...] | list[str] = DEFAULT_IGNORE,
) -> dict[str, Any]:
    return compare(primary, shadow, ignore)


def load_jsonl(path: str | Path) -> dict[str, list[dict[str, Any]]]:
    """Группирует записи по id: для одного запроса может быть несколько реплик."""
    index: dict[str, list[dict[str, Any]]] = {}
    with open(path, encoding="utf-8") as stream:
        for line in stream:
            if not line.strip():
                continue
            try:
                record = json.loads(line)
            except ValueError:
                continue
            identifier = record.get("id")
            if not identifier:
                continue
            index.setdefault(str(identifier), []).append(record)
    return index


def analyze_files(
    primary_path: str | Path,
    shadow_path: str | Path,
    ignore: tuple[str, ...] | list[str] = DEFAULT_IGNORE,
) -> dict[str, Any]:
    primary_index = load_jsonl(primary_path)
    shadow_index = load_jsonl(shadow_path)
    common = sorted(set(primary_index) & set(shadow_index))

    comparisons = [
        compare(primary_index[identifier][0], shadow_index[identifier][0], ignore)
        for identifier in common
    ]
    summary = {name: 0 for name in ("match", "minor_diff", "major_diff")}
    for item in comparisons:
        summary[item["verdict"]] += 1

    unmatched = {
        "primary": sorted(set(primary_index) - set(shadow_index)),
        "shadow": sorted(set(shadow_index) - set(primary_index)),
    }
    return {
        "version": VERSION,
        "summary": {
            "compared": len(comparisons),
            **summary,
            "unmatched_primary": len(unmatched["primary"]),
            "unmatched_shadow": len(unmatched["shadow"]),
        },
        "comparisons": comparisons,
        "unmatched": unmatched,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Сравнить primary и shadow JSONL-записи")
    parser.add_argument("--primary", required=True, help="primary.jsonl")
    parser.add_argument("--shadow", required=True, help="shadow.jsonl")
    parser.add_argument("--out", help="куда записать JSON-отчёт")
    parser.add_argument("--ignore", default="", help="дополнительные поля через запятую")
    parser.add_argument("--fail-on", choices=["none", "minor", "major"], default="none")
    parser.add_argument("--quiet", action="store_true")
    args = parser.parse_args(argv)

    ignore = tuple(DEFAULT_IGNORE) + tuple(x.strip() for x in args.ignore.split(",") if x.strip())
    try:
        report = analyze_files(args.primary, args.shadow, ignore)
    except OSError as error:
        print(f"apishadow: {error}", file=sys.stderr)
        return 2

    if args.out:
        Path(args.out).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    if not args.quiet:
        s = report["summary"]
        print(
            f"compared={s['compared']} match={s['match']} "
            f"minor={s['minor_diff']} major={s['major_diff']}"
        )
        for item in report["comparisons"]:
            if item["verdict"] != "match":
                print(f"  {item['verdict']:10} {item['method']} {item['path']} score={item['score']}")

    if args.fail_on == "major" and report["summary"]["major_diff"]:
        return 1
    if args.fail_on == "minor" and (report["summary"]["minor_diff"] or report["summary"]["major_diff"]):
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
