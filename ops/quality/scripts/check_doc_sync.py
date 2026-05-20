#!/usr/bin/env python3
"""Lightweight doc sync checks (docs/specs/documentation-spec.md change rule)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import doc_checks_common as C  # noqa: E402

MODULE_RE = re.compile(r"`(module-[a-z0-9-]+\.md)`")

SPEC_INDEXES = (
    Path("docs/specs/internals/README.md"),
    Path("docs/specs/rust/README.md"),
)


def check_index(root: Path, index_rel: Path) -> list[str]:
    failures: list[str] = []
    index_path = root / index_rel
    if not index_path.is_file():
        return [f"missing {index_rel}"]
    text = index_path.read_text(encoding="utf-8")
    listed = set(MODULE_RE.findall(text))
    spec_dir = index_path.parent
    on_disk = {p.name for p in spec_dir.glob("module-*.md")}
    for name in sorted(on_disk):
        if name not in listed:
            failures.append(
                f"{index_rel}: file {name} exists but is not linked from the module index table."
            )
    for name in sorted(listed):
        if not (spec_dir / name).is_file():
            failures.append(f"{index_rel}: table references missing file {name}")
    return failures


def main() -> int:
    root = C.repo_root()
    failures: list[str] = []
    for index_rel in SPEC_INDEXES:
        failures.extend(check_index(root, index_rel))
    if failures:
        print("check_doc_sync: FAILED", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print("check_doc_sync: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
