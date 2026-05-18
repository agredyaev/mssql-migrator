#!/usr/bin/env python3
"""Verify that backtick-quoted repo paths exist (docs/specs/documentation-spec.md)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import doc_checks_common as C  # noqa: E402

REF_RE = re.compile(r"`((?:internal|cmd|docs|ops)/[^`]+|Makefile)`")


def should_skip_path(raw: str) -> bool:
    if "*" in raw or "?" in raw or "(" in raw or ")" in raw:
        return True
    parts = raw.split("/")
    if not parts:
        return True
    last = parts[-1]
    # Skip type-like segments (e.g. internal/app.Run, internal/driver.Conn).
    if "." in last and not last.endswith((".go", ".md", ".sql", ".yml", ".yaml")):
        return True
    return False


def main() -> int:
    root = C.repo_root()
    failures: list[str] = []
    for path in C.iter_markdown_files(root):
        rel = C.rel_to_root(root, path)
        if rel.startswith("docs/templates/"):
            continue
        text = path.read_text(encoding="utf-8")
        for m in REF_RE.finditer(text):
            raw = m.group(1).strip().rstrip("/")
            if should_skip_path(raw):
                continue
            target = root / raw
            if not target.exists():
                failures.append(f"{rel}: missing path `{raw}`")
    if failures:
        print("check_doc_path_references: FAILED", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print("check_doc_path_references: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
