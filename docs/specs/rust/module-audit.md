# Module `audit`

Lifecycle: `Current`.

## Purpose

Describe **azdo_deploy_meta audit tables**: repository checksums, live managed-object fingerprints, history flush on apply, and process-local bootstrap state.

## Scope

- `crates/core/src/audit/load/mod.rs` - bootstrap, checksum load, cache flags (`tables_ensured`, history probes)
- `crates/core/src/audit/history.rs` - flush applied/adopted records, index ensure
- `crates/core/src/audit/migrations.rs` - transition migration history
- SQL: `sql/audit/bootstrap_tables.sql`, `bootstrap_index.sql`, `history_exists.sql`; `INSERT_HISTORY`/`LOAD_CHECKSUMS` are composed in `crates/core/src/sql/mod.rs` from `insert_history_header.sql` / `load_checksums_header.sql` + the shared `_object_canonical_state.sql` fingerprint block (+ `insert_history_tail.sql`)

## System context

Plan loads the latest repository checksum and compares `history.live_definition_checksum` with a fresh SQL Server fingerprint. Despite its legacy column name, that field covers all managed kinds: tables, indexes, types, sequences, synonyms, views, functions, procedures, and triggers. Apply captures the fingerprint after successful DDL and invalidates process-local checksum state.

## Interfaces and boundaries

- Public: `ensure_tables`, `flush_history`, `invalidate_audit_cache`, `invalidate_audit_cache_all`, `db_fingerprint`
- Used by: `db::plan_batch`, `apply::execute_plan`

## Assumptions and constraints

- Tables created idempotently (`IF OBJECT_ID … IS NULL`).
- Test pre-bootstrap: `crates/core/tests/common/db_reset.rs` runs bootstrap after CREATE DATABASE.
- A malformed persisted object checksum is an integrity error (exit `4`). `plan`, `validate`, `migrate`, and `baseline` stop before diff or history writes. Only `repair-checksum` may replace it.
- Module fingerprints hash `OBJECT_DEFINITION`. Non-module fingerprints hash canonical `sys.*` metadata: table columns/constraints, index shape/state, user-defined type shape, sequence bounds/options, or synonym target.
- `repair-checksum` is metadata-only. It executes no repository DDL; using it asserts that the operator has verified the current live object matches the repository.

## Nominal flow

1. `ensure_tables` or pre-bootstrap creates schema/tables.
2. Plan: plan-side `load_checksums_plan` / batched equivalent in `plan_batch`.
3. Diff: module drift selects the idempotent update path; non-module live drift or an unsupported repository change sets `plan.blocked` and action `fail`.
4. Apply: collect `HistoryRecord`; `flush_history` captures the post-DDL live fingerprint in the same object transaction where applicable.

## Verification and validation

- `crates/core/src/audit/load/mod.rs` unit tests
- `crates/core/tests/drift_e2e_integration.rs` malformed-checksum, table/index live-drift, legacy upgrade, and repair regressions
- Workflow integration audit row counts

## Off-nominal behavior and failure containment

- Failure mode: bootstrap SQL error on empty DB.
  Containment: error propagates to engine; migrate does not proceed.
- Failure mode: latest persisted checksum is not a 32-byte digest.
  Containment: return `Error::Checksum`; run `repair-checksum` to write a metadata-only replacement.
- Failure mode: live non-module state differs from the last recorded fingerprint.
  Containment: `plan`, `validate`, and `migrate` fail closed with a structural-state blocker; restore the audited shape or review and explicitly run `repair-checksum`.
- Failure mode: an upgraded history row has no live fingerprint.
  Containment: read-only commands do not mutate the audit schema; mutating bootstrap adds the nullable column, and `repair-checksum` records a verified baseline.

## Operations and recovery

- After DROP/CREATE test reset, run pre-bootstrap via `db_reset.rs`.
- Before `repair-checksum`, compare the live object with its repository SQL. The command records trust; it does not make the database match the repository.

## Open issues and non-goals

- Non-goals: fingerprints do not manage database-only objects or unsupported SQL Server resources such as partition functions.

## References

- `docs/specs/rust/module-db.md`
