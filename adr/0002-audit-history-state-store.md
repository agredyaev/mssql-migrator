# ADR-0002: Audit history is the state store; body+history commit atomically

Status: Accepted
Date: 2026-07-21

## Context

Migrator needs durable record of what applied, to skip unchanged objects and to
resume after a crash. Two hazards: (1) a checkpoint that disagrees with the DB;
(2) an applied script whose history row was lost (or vice versa) → next run
misclassifies. No separate checkpoint file should be able to lie about DB state.

## Decision

No separate checkpoint or resume-offset. State store = the audit history table
`azdo_deploy_meta.history` (bootstrapped by `sql/audit/bootstrap_tables.sql`).
Each managed object carries a SHA-256 checksum of its script body (CRLF-folded).
Idempotency: plan compares prior recorded checksum vs on-disk checksum →
`SkipUnchanged` when equal. Modules apply via `CREATE OR ALTER` (idempotent).

Non-idempotent applies (transitions, first module record) run as one atomic
unit (`crates/core/src/apply/history_write.rs::apply_in_tx`):
`BEGIN TRANSACTION` (with `SET XACT_ABORT ON`) → script body → `ASSERT_OPEN_TX`
→ history INSERT → `COMMIT`. Any failure rolls back. `ASSERT_OPEN_TX` guards
against a body that embeds its own COMMIT/ROLLBACK (which would let the history
row autocommit outside the intended transaction).

Apply order (`crates/core/src/apply/mod.rs`): ensure tables → ensure history
index → schemas → table transitions (before objects, so dependents see new
columns) → modules via `CREATE OR ALTER`.

## Consequences

- Script body and its history row commit together or not at all. No window where
  an applied script lacks its audit row, or an audit row lies about an object.
- Resume after `kill -9`: each object is fully applied+recorded or untouched;
  next run re-plans from live DB state under the advisory lock and continues.
  Verified by `crates/core/tests/chaos_kill_mid_apply_test.rs`.
- No retry duplication: connect-phase retries only (ADR context); no
  statement-level retry, so no double-apply.
- Cost: apply is not one all-or-nothing transaction across all objects; recovery
  relies on fail-fast + per-object atomicity + idempotent re-run.
