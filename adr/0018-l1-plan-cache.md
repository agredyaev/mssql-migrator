# ADR-0018: Remove the L1 filesystem plan cache

Status: Accepted
Date: 2026-07-21
Updated: 2026-07-24

## Context

The L1 cache and in-process warm snapshot could only serve a workspace with no
managed objects. Production and E2E layouts contain managed objects and must
refresh live SQL Server state.

Compatibility shims kept dead cache configuration, timing fields, tests, and
profilers after the cache stopped affecting runtime behavior.

## Decision

Remove the L1 cache, warm snapshot, compatibility symbols, configuration,
timing fields, fixtures, and profilers. `run_plan_db_phase` always obtains
catalog and audit state from SQL Server.

## Assumptions and constraints

- SQL Server is the authority for live catalog state.
- `rmigd` may keep a TDS connection warm; it does not cache plan state.

## Consequences

- Managed-workspace behavior is unchanged.
- Empty workspaces perform SQL work instead of returning a local cache hit.
- `RMIG_L1_CACHE_DIR`, `RMIG_INTEGRATION_WARM_SNAPSHOT`, and
  `l1_cache_hit` are no longer accepted or emitted.
- Existing `.rmig/cache` directories are unused and may be deleted.

## Verification

- `cargo test --workspace --all-targets --all-features`
- `make plan-db-perf`
- `make e2e-all`

## Non-goals

- Filesystem plan caching
- Process-global plan snapshots
