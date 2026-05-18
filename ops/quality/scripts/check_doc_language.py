#!/usr/bin/env python3
"""Basic language checks for durable Markdown (docs/specs/nasa-document-spec.md)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import doc_checks_common as C  # noqa: E402

CYRILLIC_RE = re.compile(r"[\u0400-\u04FF]")


def main() -> int:
    root = C.repo_root()
    failures: list[str] = []
    for path in C.iter_markdown_files(root):
        rel = C.rel_to_root(root, path)
        if rel in C.LANGUAGE_EXEMPT:
            continue
        text = path.read_text(encoding="utf-8")
        if CYRILLIC_RE.search(text):
            failures.append(f"{rel}: contains Cyrillic characters (canonical docs must be English).")
    if failures:
        print("check_doc_language: FAILED", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print("check_doc_language: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
