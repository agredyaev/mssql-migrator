# Documentation validation scripts (`ops/quality/scripts/`)

Lifecycle: `Current`.

## Purpose

Implement the **Verification Expectations** in `docs/specs/nasa-document-spec.md` with small, dependency-free **Python 3** scripts so maintainers can validate durable Markdown locally or in CI.

## Scope

- `ops/quality/scripts/doc_checks_common.py` — shared paths and exemption lists
- `ops/quality/scripts/check_doc_structure.py` — NASA-style heading coverage
- `ops/quality/scripts/check_doc_context.py` — lifecycle line
- `ops/quality/scripts/check_doc_path_references.py` — backtick path existence
- `ops/quality/scripts/check_doc_language.py` — English-only (no Cyrillic) for durable docs
- `ops/quality/scripts/check_doc_sync.py` — `docs/specs/internals` and `docs/specs/rust` index vs `module-*.md` files

## System context

Repository root is detected from `Path(__file__).resolve().parents[3]` unless **`REPO_ROOT`** is set in the environment. Scripts insert their own directory on `sys.path` to import `doc_checks_common`.

## Interfaces and boundaries

- Inputs: UTF-8 Markdown under `docs/`, `README.md`, `AGENTS.md`
- Outputs: stdout status lines; stderr details on failure; process exit code `0` or `1`
- Boundaries: these checks are **heuristic** (they do not parse full Markdown AST).

## Assumptions and constraints

- Assumptions: `python3` is available on the runner (`make doc-check`).
- Constraints: `docs/templates/document-template.md` is exempt from full heading and lifecycle rules because it contains instructional placeholders.

## Nominal flow

1. From the repository root, run `make doc-check`.
2. Or run each script: `python3 ops/quality/scripts/check_doc_structure.py`, etc.

## Off-nominal behavior and failure containment

- Failure mode: missing `python3`.
  Containment: install Python 3 or run checks only in environments where it exists.
- Failure mode: false positive on path references.
  Containment: narrow backtick usage to real paths or adjust `REF_RE` in `check_doc_path_references.py`.

## Verification and validation

- `make doc-check`
- `python3 ops/quality/scripts/check_doc_structure.py` (repeat for each `check_doc_*.py`)

## Operations and recovery

- When adding a new `docs/specs/internals/module-*.md` or `docs/specs/rust/module-*.md`, add a table row in the matching `README.md` so `check_doc_sync.py` stays green.

## Open issues and non-goals

- Open issues: no automated link checker for arbitrary URLs.
- Non-goals: these scripts do not replace spelling or prose-style review.

## References

- `docs/specs/nasa-document-spec.md`
- `docs/specs/documentation-spec.md`
- `docs/templates/document-template.md`
- `Makefile` (`doc-check` target)
