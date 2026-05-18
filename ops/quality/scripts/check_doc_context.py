#!/usr/bin/env python3
"""Verify lifecycle line in durable docs (docs/specs/nasa-document-spec.md)."""

from __future__ import annotations

import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import doc_checks_common as C  # noqa: E402

LIFECYCLE_RE = re.compile(
    r"^Lifecycle:\s*`(?:Planned|Current|Target|Historical|Generated)`\s*\.\s*$",
    re.M,
)


def main() -> int:
    root = C.repo_root()
    failures: list[str] = []
    for path in C.iter_markdown_files(root):
        rel = C.rel_to_root(root, path)
        if rel in C.CONTEXT_EXEMPT:
            continue
        head = "\n".join(path.read_text(encoding="utf-8").splitlines()[:120])
        if not LIFECYCLE_RE.search(head):
            failures.append(f"{rel}: expected line like: Lifecycle: `Current`.")
    if failures:
        print("check_doc_context: FAILED", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print("check_doc_context: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
