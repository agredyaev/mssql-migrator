# ADR-0016: Advisory lock for single-writer mutual exclusion; plan under lock

Status: Accepted
Date: 2026-07-21

## Context

Two `rmig migrate` runs against the same database concurrently could interleave
DDL, corrupt the audit history, or race the plan against another writer's apply
(stale plan). Need mutual exclusion per target database, and the plan must be
consistent with the state it will apply to.

## Decision

Serialize per database with a SQL Server application lock. `sql/lock/acquire.sql`:
`sp_getapplock @Resource='reporting_layer_migration', @LockMode='Exclusive',
@LockOwner='Session', @LockTimeout=@p1`.

Both plan AND apply run inside the lock (`crates/core/src/engine/apply_run.rs`,
`run_locked`): acquire → `locked_body` (plan_phase + apply_plan) → always
`release_after_body` (released even on body failure — regression BG-001, gate
`scripts/check-advisory-lock-release.sh`). Lock timeout →
`Error::LockTimeout` → exit `7` (`EXIT_LOCK_TIMEOUT`).

`Session` owner (not transaction) so the lock spans the multi-statement
plan+apply. Released on explicit release or on connection close (covers SIGINT
drop, ADR-0012).

## Consequences

- One migrator writes a given database at a time; the plan cannot go stale under
  a concurrent apply. Verified by `advisory_lock_guard_test`,
  `advisory_lock_rmigd_test`, `chaos_kill_mid_apply_test` (concurrent migrate →
  one exits 7 / serializes exactly once).
- The lock is the correctness reason `rmigd` holds one warm connection per client
  session (ADR-0008) — a Session-owned lock cannot be split across connections.
- No cross-database global lock: different databases migrate in parallel.
- Deadlock-victim (`sp_getapplock` `-3`) surfaces as an error, not retried — no
  statement-level retry (ADR-0022).
