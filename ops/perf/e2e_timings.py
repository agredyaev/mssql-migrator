#!/usr/bin/env python3
"""E2e phase timings from last run artifacts vs committed baselines."""

from __future__ import annotations

import json
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ART = ROOT / "ops" / "perf" / "artifacts"
BASELINE_DIR = ROOT / "crates" / "core" / "tests" / "testdata" / "e2e"

PLAN_SCENARIOS = [
    "empty_db_plan",
    "warm_db_plan",
    "skip_unchanged_plan",
    "catalog_cache_plan",
]

TIMING_ROWS: list[tuple[str, str]] = [
    ("plan_wall_ms", "Plan wall"),
    ("cli_wall_ms", "CLI wall"),
    ("connect_ms", "Connect"),
    ("scan_ms", "Scan layout"),
    ("parallel_wall_ms", "Parallel DB wall"),
    ("ensure_ms", "Ensure audit tables"),
    ("checksums_ms", "Load checksums"),
    ("inspect_ms", "Catalog inspect"),
    ("audit_ms", "Audit composite"),
    ("diff_ms", "In-process diff"),
    ("plan_db_path", "Plan DB path"),
    ("plan_db_query_ms", "Plan DB query ms"),
    ("plan_db_round_trips", "Plan DB round trips"),
    ("l1_cache_hit", "L1 cache hit"),
]


def load_json(path: Path) -> dict:
    with path.open(encoding="utf-8") as f:
        return json.load(f)


def fmt_val(key: str, v) -> str:
    if v is None:
        return "n/a"
    if key == "plan_db_path":
        return str(v) if v else "n/a"
    if isinstance(v, bool):
        return "yes" if v else "no"
    if isinstance(v, (int, float)):
        return str(int(v))
    return str(v)


def delta_str(base_v, run_v) -> str:
    if base_v is None and run_v is None:
        return "n/a"
    b = 0 if base_v is None else int(base_v)
    r = 0 if run_v is None else int(run_v)
    if b == r:
        return "="
    d = r - b
    if b == 0:
        return f"{d:+d}ms"
    return f"{d:+d}ms ({d / b * 100:+.0f}%)"


def section(scenario: str, baseline: dict, run: dict) -> list[str]:
    bt = baseline.get("timings") or {}
    rt = run.get("timings") or {}
    lines = [
        f"### {scenario}",
        "",
        f"**Actions:** baseline `{baseline.get('action_counts')}` · run `{run.get('action_counts')}`",
        "",
        "| Phase | Baseline | Run | Delta |",
        "|-------|----------|-----|-------|",
    ]
    shown = {k for k, _ in TIMING_ROWS}
    for key, label in TIMING_ROWS:
        bv = bt.get(key)
        rv = rt.get(key)
        if bv is None and rv is None:
            continue
        lines.append(
            f"| {label} (`{key}`) | {fmt_val(key, bv)} | {fmt_val(key, rv)} | {delta_str(bv, rv)} |"
        )
    for key in sorted((set(bt) | set(rt)) - shown):
        lines.append(
            f"| _extra_ `{key}` | {fmt_val(key, bt.get(key))} | {fmt_val(key, rt.get(key))} | "
            f"{delta_str(bt.get(key), rt.get(key))} |"
        )
    lines.append("")
    return lines


def main() -> int:
    out_path = ART / "e2e_timings_report.md"
    rows: list[tuple[str, dict, dict]] = []
    missing: list[str] = []

    for scenario in PLAN_SCENARIOS:
        base_path = BASELINE_DIR / f"e2e_baseline_{scenario}.json"
        run_path = ART / f"e2e_{scenario}.json"
        if not base_path.is_file() or not run_path.is_file():
            missing.append(scenario)
            continue
        rows.append((scenario, load_json(base_path), load_json(run_path)))

    if missing:
        print(f"missing data for: {', '.join(missing)}", file=sys.stderr)
        print("run: make e2e or make e2e-all", file=sys.stderr)

    lines = [
        "# E2e phase timings (baseline vs run)",
        "",
        f"Generated: {datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')}",
        "",
        "Baseline: `crates/core/tests/testdata/e2e/e2e_baseline_<scenario>.json`",
        "Run: `ops/perf/artifacts/e2e_<scenario>.json` (from `make e2e` / `make e2e-all`).",
        "",
    ]
    if rows:
        lines.extend(["## Summary (plan_wall_ms)", "", "| Scenario | Baseline | Run | Delta |", "|----------|----------|-----|-------|"])
        for scenario, base, run in rows:
            b = int((base.get("timings") or {}).get("plan_wall_ms") or 0)
            r = int((run.get("timings") or {}).get("plan_wall_ms") or 0)
            lines.append(f"| `{scenario}` | {b} ms | {r} ms | {r - b:+d} ms |")
        lines.extend(["", "## Per-scenario breakdown", ""])
        for scenario, base, run in rows:
            lines.extend(section(scenario, base, run))

    out_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(out_path)
    print("\n".join(lines))
    return 0 if rows else 1


if __name__ == "__main__":
    raise SystemExit(main())
