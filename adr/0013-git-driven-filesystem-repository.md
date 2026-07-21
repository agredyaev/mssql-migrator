# ADR-0013: Git-driven filesystem repository as the source of truth

Status: Accepted
Date: 2026-07-21

## Context

Need a source of truth for the desired schema of a SQL Server reporting layer,
diffable and reviewable per change, versioned, CI-friendly. Candidates: a
declarative DSL, a model DB compared by a diff tool, or a tree of raw `.sql`
authored by the team under version control.

## Decision

Source of truth = a git repository of raw `.sql` object files. Tool is stateless:
it matches the declared filesystem layout against SQL Server's live catalog. No
model database, no DSL.

Layout (`docs/repository-contract.md`):
- Root `RM_SQL_ROOT`. Each top-level directory = a database name (multi-database
  catalog discovered from the tree).
- Object path `<database>/<schema>/<object-type>/<object-name>.sql`. Object types
  = the `KINDS` list in `crates/core/src/scan/parse_object.rs` (`tables`, `views`,
  `procedures`, `functions`, `triggers`, `indexes`, `types`, `sequences`,
  `synonyms`).
- `_migrations/` paths → table transitions (ADR-0015). `checks/` paths →
  non-deploying checks.
- Object keys normalized to lowercase `schema/kind/name` (case-insensitive).
  Files sorted → deterministic plan.
- Paths validated: UTF-8 only; no `.`, `..`, `/`, `\`, NUL in components; symlinks
  skipped; generated idents ≤128 chars.

git provides history, review, and the change-set (ADR-0017 uses git-delta to
scope inspection).

## Consequences

- Every schema change is a reviewable `.sql` diff in version control; the author
  owns SQL correctness, the tool owns path/identity validity + safe apply.
- Deterministic, reproducible plans from a given tree state.
- The whole model rests on this: ownership (ADR-0014), transitions (ADR-0015),
  checksums (ADR-0021), git-delta (ADR-0017) all derive from "the tree is the
  desired state."
- Not a data/ETL migrator: no row copy, no bulk insert, no type mapping. Scope is
  schema/DDL only.
