#!/usr/bin/env python3
"""Summarize dhat v2 heap JSON as an allocation call-tree (pprof-style top/cum).

Usage:
  python3 ops/perf/dhat_alloc_tree.py [path/to/dhat-heap.json] [--iterations 20]

Writes human-readable report to stdout. Categories map to rmig hot-path CASE-* ids in
docs/data-oriented-layout-policy.md.

Phases (by stack symbols in plan_diff_dhat*.rs):
  setup  - bench_setup / bench_scan_setup / skip_heavy_workspace / scan_fixture / scan_root
  warm   - bench_warm (first compute_diff_into; plan Vec resize)
  loop   - bench_loop (warmed compute_diff_into iterations; headline per-iter metric)
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

RMIG_MARKERS = (
    "migrator_core::",
    "plan_diff_dhat",
)

SETUP_MARKERS = (
    "bench_setup",
    "bench_scan_setup",
    "skip_heavy_workspace",
    "table_heavy_workspace",
    "scan_fixture_workspace",
    "scan_root",
    "adopt_dense_entries",
)

WARM_MARKERS = ("bench_warm",)

LOOP_MARKERS = ("bench_loop",)


def load_dhat(path: Path) -> dict:
    with path.open(encoding="utf-8") as f:
        return json.load(f)


def frame_name(ftbl: list[str], idx: int) -> str:
    if idx < 0 or idx >= len(ftbl):
        return f"<frame {idx}>"
    return ftbl[idx]


def stack_frames(ftbl: list[str], fs: list[int]) -> list[str]:
    return [frame_name(ftbl, i) for i in fs]


def stack_phase(stack: list[str]) -> str:
    joined = " ".join(stack)
    if any(m in joined for m in LOOP_MARKERS):
        return "loop"
    if any(m in joined for m in WARM_MARKERS):
        return "warm"
    if any(m in joined for m in SETUP_MARKERS):
        return "setup"
    if "compute_diff" in joined:
        return "warm"
    return "setup"


def deepest_rmig_frame(stack: list[str]) -> str | None:
    for frame in reversed(stack):
        if any(m in frame for m in RMIG_MARKERS):
            return frame
    return None


def categorize(stack: list[str], phase: str) -> str:
    joined = " ".join(stack)
    if phase == "setup":
        if "HashMap" in joined and "insert" in joined:
            return "setup:HashMap_insert"
        if "skip_heavy_workspace" in joined or "table_heavy_workspace" in joined:
            return "setup:bench_support"
        if "scan_fixture" in joined or "scan_root" in joined:
            return "setup:scan_fixture"
        return "setup:other"

    if "push_planned" in joined or "fill_planned" in joined:
        return f"{phase}:push_planned"
    if "decide_object" in joined or "diff_decide" in joined:
        return f"{phase}:decide_object"
    if "compute_diff" in joined:
        if "paths_by_table" in joined or "transitions" in joined:
            return f"{phase}:transitions_map"
        if "ensure_plan_objects" in joined or "resize_with" in joined:
            return f"{phase}:plan_resize"
        if "PlannedSchema" in joined or "schema.name" in joined:
            return f"{phase}:compute_diff_schema"
        return f"{phase}:compute_diff_other"
    if "String" in joined and "clone" in joined.lower():
        return f"{phase}:string_clone"
    if "Vec" in joined and ("push" in joined or "with_capacity" in joined):
        return f"{phase}:vec_growth"
    return f"{phase}:uncategorized"


CASE_MAP = {
    "setup:bench_support": "setup (exclude)",
    "setup:scan_fixture": "setup scan (CASE-1 tail)",
    "setup:HashMap_insert": "setup (exclude)",
    "setup:string_interner": "CASE-6 setup",
    "setup:other": "setup",
    "warm:plan_resize": "CASE-7 one-time",
    "warm:transitions_map": "CASE-4 warm",
    "warm:compute_diff_other": "CASE-1 warm",
    "loop:transitions_map": "CASE-4",
    "loop:push_planned": "CASE-7, CASE-6",
    "loop:compute_diff_other": "CASE-1",
    "loop:uncategorized": "n/a",
}


def fmt_bytes(n: int) -> str:
    if n >= 1_000_000:
        return f"{n / 1_000_000:.2f} MB"
    if n >= 1_000:
        return f"{n / 1_000:.1f} KB"
    return f"{n} B"


def short_frame(frame: str) -> str:
    m = re.match(r"0x[0-9a-f]+: (.+) \((.+)\)", frame)
    if m:
        sym, loc = m.group(1), m.group(2)
        return f"{sym} ({loc})"
    return frame


def report(data: dict, iterations: int) -> str:
    ftbl: list[str] = data["ftbl"]
    pps: list[dict] = data["pps"]

    total_tb = sum(int(p["tb"]) for p in pps)
    by_category: dict[str, int] = defaultdict(int)
    by_phase: dict[str, int] = defaultdict(int)
    by_rmig_frame: dict[str, int] = defaultdict(int)
    by_rmig_any: dict[str, int] = defaultdict(int)
    by_flat_leaf: dict[str, int] = defaultdict(int)

    for entry in pps:
        tb = int(entry["tb"])
        stack = stack_frames(ftbl, entry["fs"])
        phase = stack_phase(stack)
        by_phase[phase] += tb
        cat = categorize(stack, phase)
        by_category[cat] += tb
        rmig = deepest_rmig_frame(stack)
        if rmig:
            by_rmig_frame[short_frame(rmig)] += tb
        for frame in stack:
            if any(m in frame for m in RMIG_MARKERS) and "plan_diff_dhat::main" not in frame:
                by_rmig_any[short_frame(frame)] += tb
        if stack:
            by_flat_leaf[short_frame(stack[-1])] += tb

    setup_bytes = by_phase.get("setup", 0)
    warm_bytes = by_phase.get("warm", 0)
    loop_bytes = by_phase.get("loop", 0)
    loop_per_iter = loop_bytes / iterations if iterations else loop_bytes

    lines: list[str] = []
    lines.append("# Rust dhat allocation summary")
    lines.append(f"# total allocated (tb sum): {fmt_bytes(total_tb)} ({total_tb} B)")
    lines.append(f"# loop iterations: {iterations}")
    lines.append(f"# setup (one-time): {fmt_bytes(setup_bytes)}")
    lines.append(f"# warm (first compute_diff / plan resize): {fmt_bytes(warm_bytes)}")
    lines.append(
        f"# loop (warmed compute_diff x{iterations}): {fmt_bytes(loop_bytes)} "
        f"({int(loop_per_iter)} B/iter)"
    )
    lines.append("")

    lines.append("## Phase totals")
    lines.append(f"{'Phase':<12} {'Bytes':>14} {'Share':>8}")
    for phase in ("setup", "warm", "loop"):
        b = by_phase.get(phase, 0)
        share = (100.0 * b / total_tb) if total_tb else 0
        lines.append(f"{phase:<12} {fmt_bytes(b):>14} {share:7.1f}%")
    lines.append("")

    lines.append("## Top categories (loop phase only)")
    lines.append(f"{'Category':<36} {'Total':>12} {'B/iter':>12} {'Share':>7}  CASE-*")
    loop_cats = [(k, v) for k, v in by_category.items() if k.startswith("loop:")]
    loop_cats.sort(key=lambda x: -x[1])
    for cat, b in loop_cats:
        share = (100.0 * b / loop_bytes) if loop_bytes else 0
        lines.append(
            f"{cat:<36} {fmt_bytes(b):>12} {fmt_bytes(int(b / iterations)):>12} "
            f"{share:6.1f}%  {CASE_MAP.get(cat, 'n/a')}"
        )
    if not loop_cats:
        lines.append("(no loop-phase allocations detected)")
    lines.append("")

    lines.append("## Top rmig frames (loop phase)")
    lines.append(f"{'Frame':<72} {'Bytes':>12} {'Share':>7}")
    loop_rmig: dict[str, int] = defaultdict(int)
    for entry in pps:
        stack = stack_frames(ftbl, entry["fs"])
        if stack_phase(stack) != "loop":
            continue
        rmig = deepest_rmig_frame(stack)
        if rmig:
            loop_rmig[short_frame(rmig)] += int(entry["tb"])
    for frame, b in sorted(loop_rmig.items(), key=lambda x: -x[1])[:15]:
        share = 100.0 * b / loop_bytes if loop_bytes else 0
        lines.append(f"{frame[:72]:<72} {fmt_bytes(b):>12} {share:6.1f}%")
    lines.append("")

    lines.append("## Top rmig frames (deepest in stack, all phases)")
    lines.append(f"{'Frame':<72} {'Bytes':>12} {'Share':>7}")
    rmig_sorted = sorted(by_rmig_frame.items(), key=lambda x: -x[1])[:15]
    for frame, b in rmig_sorted:
        share = 100.0 * b / total_tb if total_tb else 0
        lines.append(f"{frame[:72]:<72} {fmt_bytes(b):>12} {share:6.1f}%")
    lines.append("")

    lines.append("## Top leaf alloc frames (flat)")
    lines.append(f"{'Frame':<72} {'Bytes':>12}")
    leaf_sorted = sorted(by_flat_leaf.items(), key=lambda x: -x[1])[:10]
    for frame, b in leaf_sorted:
        lines.append(f"{frame[:72]:<72} {fmt_bytes(b):>12}")

    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description="dhat heap JSON → alloc call-tree summary")
    parser.add_argument(
        "json_path",
        nargs="?",
        default="ops/perf/artifacts/dhat_heap.json",
        help="Path to dhat-heap.json",
    )
    def positive_int(v: str) -> int:
        n = int(v)
        if n < 1:
            raise argparse.ArgumentTypeError("must be a positive integer")
        return n

    parser.add_argument(
        "--iterations",
        type=positive_int,
        default=20,
        help="bench_loop iteration count in dhat bench (>= 1)",
    )
    args = parser.parse_args()

    path = Path(args.json_path)
    if not path.is_file():
        alt = Path("crates/core-dev/dhat-heap.json")
        if alt.is_file():
            path = alt
        else:
            print(f"error: dhat json not found: {args.json_path}", file=sys.stderr)
            return 1

    data = load_dhat(path)
    print(report(data, args.iterations), end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
