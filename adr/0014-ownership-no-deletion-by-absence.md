# ADR-0014: Ownership model — managed/unmanaged/orphaned, no deletion by absence

Status: Accepted
Date: 2026-07-21

## Context

Live catalog holds objects the repo does not (other teams' tables, tooling
objects, objects removed from the repo). A diff tool that drops "anything not in
the source" is catastrophic against a shared production catalog. Removing a `.sql`
file must never mean "drop that object."

## Decision

Three ownership states (`docs/repository-contract.md`):
- **Managed**: has a `.sql` file in the tree. The only kind any command acts on.
- **Unmanaged**: exists in the live catalog, no file. The diff never enumerates
  it → never created, altered, or dropped.
- **Orphaned**: was managed, file since removed. Treated exactly like unmanaged —
  preserved.

The diff iterates ONLY workspace (repository) objects. Absence from the tree can
never produce a destructive operation. Deletion is not inferred from absence: an
intentional drop must be authored explicitly as a table transition script under
`_migrations/` (ADR-0015).

First adoption: `baseline` records repository objects that already exist in the
database. `migrate` does the same only when the process operator sets
`RMIG_ALLOW_ADOPT=1`; otherwise it fails closed with exit `10`. Database-only
objects are always left unmanaged.

## Consequences

- Safe by default against shared catalogs: the tool cannot delete an object it
  did not create, and cannot delete by omission.
- A removed repo file leaves its DB object standing (orphaned) — operators must
  drop deliberately via an authored transition, never accidentally.
- Name-only adoption during `migrate` requires a process-level opt-in. Repository
  TOML cannot grant it.
- Cost: the tool never reconciles unmanaged drift/cruft; out of scope by design.
- Duplicate keys (two files → same `schema/kind/name`, incl. case-only) are a
  hard scan error (exit `8`) before any DB contact.

## Verification

- `crates/core/tests/unmanaged_objects_test.rs`
- `crates/core/tests/apply_integrity_integration.rs`
- `crates/core/tests/existing_db_adoption_integration.rs`
- `make check-e2e`

## References

- `docs/repository-contract.md`
- `docs/migration-flow.md`
- `adr/0020-config-env-over-toml-secrets-env-only.md`
