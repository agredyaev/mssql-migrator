"""Shared helpers for `ops/quality/scripts/check_doc_*.py` (see `docs/specs/nasa-document-spec.md`)."""

from __future__ import annotations

import os
from pathlib import Path


def repo_root() -> Path:
    if os.environ.get("REPO_ROOT"):
        return Path(os.environ["REPO_ROOT"]).resolve()
    # ops/quality/scripts/<module>.py -> parents[3] == repository root
    return Path(__file__).resolve().parents[3]


def iter_markdown_files(root: Path) -> list[Path]:
    out: list[Path] = []
    for rel in ("README.md", "AGENTS.md"):
        p = root / rel
        if p.is_file():
            out.append(p)
    docs = root / "docs"
    if docs.is_dir():
        for p in sorted(docs.rglob("*.md")):
            if p.is_file():
                out.append(p)
    return out


def rel_to_root(root: Path, path: Path) -> str:
    try:
        return path.relative_to(root).as_posix()
    except ValueError:
        return path.as_posix()


# Meta docs and placeholders: not required to match full NASA heading matrix.
STRUCTURE_EXEMPT: frozenset[str] = frozenset(
    {
        "AGENTS.md",
        "docs/templates/document-template.md",
        "docs/specs/documentation-spec.md",
        "docs/specs/nasa-document-spec.md",
    }
)

CONTEXT_EXEMPT: frozenset[str] = frozenset(
    {
        "docs/templates/document-template.md",
    }
)

LANGUAGE_EXEMPT: frozenset[str] = frozenset(
    {
        "docs/templates/document-template.md",
    }
)
