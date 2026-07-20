#!/usr/bin/env python3
"""Verify NASA-style section headings for durable Markdown (see docs/templates/document-template.md)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import doc_checks_common as C  # noqa: E402

# Each pattern must match at least one ## heading line (case-insensitive).
HEADING_TESTS: list[tuple[str, re.Pattern[str]]] = [
    ("Purpose", re.compile(r"(?mi)^##\s+.*\bpurpose\b")),
    ("Scope", re.compile(r"(?mi)^##\s+.*\bscope\b")),
    ("System context", re.compile(r"(?mi)^##\s+.*system\s+context\b")),
    ("Interfaces / boundaries", re.compile(r"(?mi)^##\s+.*(interfaces?|boundary)\b")),
    ("Assumptions / constraints", re.compile(r"(?mi)^##\s+.*(assumptions?|constraints?)\b")),
    ("Nominal flow", re.compile(r"(?mi)^##\s+.*nominal\s+flow\b")),
    ("Off-nominal / failure", re.compile(r"(?mi)^##\s+.*(off-nominal|off nominal|failure\s+containment)\b")),
    ("Verification / validation", re.compile(r"(?mi)^##\s+.*(verification|validation)\b")),
    ("Operations / recovery", re.compile(r"(?mi)^##\s+.*(operations?|recovery)\b")),
    ("Open issues / non-goals", re.compile(r"(?mi)^##\s+.*(open\s+issues?|non-goals?)\b")),
    ("References", re.compile(r"(?mi)^##\s+.*\breferences?\b")),
]


def main() -> int:
    root = C.repo_root()
    failures: list[str] = []
    for path in C.iter_markdown_files(root):
        rel = C.rel_to_root(root, path)
        if rel in C.STRUCTURE_EXEMPT:
            continue
        text = C.strip_fenced_blocks(path.read_text(encoding="utf-8"))
        missing = [name for name, rx in HEADING_TESTS if not rx.search(text)]
        if missing:
            failures.append(f"{rel}: missing sections → {', '.join(missing)}")
    if failures:
        print("check_doc_structure: FAILED", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print("check_doc_structure: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
