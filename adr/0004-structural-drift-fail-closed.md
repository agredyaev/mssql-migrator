# ADR-0004: Structural live drift blocks the plan (fail-closed)

Status: Accepted
Date: 2026-07-21

## Context

Someone can ALTER a managed object directly in the live DB (out of band). Then
repo checksum still matches the audited checksum → naive skip → the manual drift
persists and hides. For a table (structural object), there is no safe in-place
DDL the tool can synthesize to reconcile — the reconciliation must be an authored
transition script. Silent skip of a drifted structural object = data-shape risk.

## Decision

Plan captures, per managed object, a fingerprint of its live definition and
compares to the fingerprint recorded at last apply
(`history.live_definition_checksum`; despite the legacy column name it covers all
managed kinds). Comparison in `crates/core/src/plan/scenario_resolve.rs`.

When live definition differs from audited (drift):
- Module (view/proc/function/trigger): re-emit via `CREATE OR ALTER`
  (`ModuleUpdate`) — safe, idempotent.
- Non-module structural object (table/index/type/sequence/synonym):
  `PlanScenario::LiveStructuralDriftBlocked` → `Action::Fail` → `plan.blocked` →
  exit code `10` (`EXIT_PLAN_BLOCKED`). Introduced in commit `ddbad29`.

Escape hatch: `repair-checksum` captures a verified live baseline (metadata-only,
runs no repo DDL — asserts operator verified live == repo).

## Consequences

- Out-of-band structural change is caught and stops the run, not silently
  overwritten or ignored. Verified by `crates/core/tests/drift_e2e_integration.rs`,
  `drift_offline_test.rs`.
- `plan`/`validate`/`migrate` all surface the block; non-json blocked runs print
  the blocker (`crates/cli/src/main.rs`).
- Cost: computing the fingerprint per object is O(catalog). See ADR-0005 (made
  incremental) and ADR-0006 (why the fingerprint is full, not a cheap shortcut).
