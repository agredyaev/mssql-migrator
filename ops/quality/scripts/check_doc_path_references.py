#!/usr/bin/env python3
"""Verify that backtick-quoted repo paths exist (docs/specs/documentation-spec.md)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import doc_checks_common as C  # noqa: E402

REF_RE = re.compile(
    r"`((?:internal|cmd|docs|ops|crates|scripts|sql|\.github)/[^`]+|Makefile)`"
)


def should_skip_path(raw: str) -> bool:
    # Globs, alternations, ellipses, and free text are not exact paths.
    if any(ch in raw for ch in "*?() ") or "..." in raw:
        return True
    return not raw.split("/")


def normalize(raw: str) -> str:
    # `path.rs:123` line references point at the file.
    head, _, tail = raw.rpartition(":")
    if head and tail.replace("-", "").isdigit():
        return head
    return raw


def main() -> int:
    root = C.repo_root()
    failures: list[str] = []
    for path in C.iter_markdown_files(root):
        rel = C.rel_to_root(root, path)
        if rel.startswith("docs/templates/"):
            continue
        text = path.read_text(encoding="utf-8")
        for m in REF_RE.finditer(text):
            raw = normalize(m.group(1).strip().rstrip("/"))
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
