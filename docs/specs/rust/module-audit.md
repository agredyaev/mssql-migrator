# Module `audit`

Lifecycle: `Current`.

## Purpose

Describe **azdo_deploy_meta audit tables**: bootstrap DDL, checksum history load, history flush on apply, process-local bootstrap cache.

## Scope

- `crates/core/src/audit/load/mod.rs` - bootstrap, checksum load, cache flags (`tables_ensured`, history probes)
- `crates/core/src/audit/history.rs` - flush applied/adopted records, index ensure
- `crates/core/src/audit/migrations.rs` - transition migration history
- SQL: `sql/audit/bootstrap_tables.sql`, `bootstrap_index.sql`, `load_checksums_openjson.sql`

## System context

Plan phase loads checksums via `LOAD_CHECKSUMS` OpenJSON batch. Apply phase writes history and invalidates process-local checksum cache (not bootstrap cache unless full DB reset).

## Interfaces and boundaries

- Public: `ensure_tables`, `flush_history`, `invalidate_audit_cache`, `invalidate_audit_cache_all`, `db_fingerprint`
- Used by: `db::plan_batch`, `apply::execute_plan`

## Assumptions and constraints

- Tables created idempotently (`IF OBJECT_ID … IS NULL`).
- Test pre-bootstrap: `crates/core/tests/common/db_reset.rs` runs bootstrap after CREATE DATABASE.
- A malformed persisted object checksum is an integrity error (exit `4`). `plan`, `validate`, `migrate`, and `baseline` stop before diff or history writes. Only `repair-checksum` may replace it.

## Nominal flow

1. `ensure_tables` or pre-bootstrap creates schema/tables.
2. Plan: plan-side `load_checksums_plan` / batched equivalent in `plan_batch`.
3. Apply: collect `HistoryRecord`, `flush_history`, `ensure_history_index`.

## Verification and validation

- `crates/core/src/audit/load/mod.rs` unit tests
- `crates/core/tests/drift_e2e_integration.rs` malformed-checksum and repair regression
- Workflow integration audit row counts

## Off-nominal behavior and failure containment

- Failure mode: bootstrap SQL error on empty DB.
  Containment: error propagates to engine; migrate does not proceed.
- Failure mode: latest persisted checksum is not a 32-byte digest.
  Containment: return `Error::Checksum`; run `repair-checksum` to write a metadata-only replacement.

## Operations and recovery

- After DROP/CREATE test reset, run pre-bootstrap via `db_reset.rs`.

## Open issues and non-goals

- Non-goals: audit module does not compute diffs.

## References

- `docs/specs/rust/module-db.md`
- `docs/specs/rust/module-db.md`
