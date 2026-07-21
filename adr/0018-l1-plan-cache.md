# ADR-0018: L1 filesystem cache for plan acceleration, invalidated on apply

Status: Accepted
Date: 2026-07-21

## Context

Repeated plans against an unchanged repo re-fetch the same checksums and catalog
state. A cache could skip that. But a cache that outlives an apply, or that is
trusted while the live DB could have drifted, would produce a wrong plan.

## Decision

An on-disk L1 cache stores `{layout_digest → (checksums, catalog_state)}` under
`.rmig/cache` (`crates/core/src/cache/l1.rs`). Keyed by the repo layout digest.
It stores checksums + catalog state, never bodies.

Trust is gated hard: the L1 short-circuit that returns a cached plan without
touching the DB fires ONLY for an empty workspace
(`plan_snapshot.rs`, `requires_live_state = object_count != 0`). Any non-empty
catalog re-queries live, because a top-level snapshot cannot prove SQL Server was
not changed out of band (ADR-0004). The cache otherwise serves as a within-run
acceleration for catalog structure, not a drift oracle.

Invalidated after every apply (`apply/mod.rs`: `invalidate_audit_cache`,
`db::invalidate`, `l1.invalidate_all`). Not git-tracked.

## Consequences

- Cache never masks out-of-band drift: unchanged-catalog trust is the exact case
  ADR-0004 forbids, so it is disabled for non-empty catalogs. ADR-0005 makes
  drift cheap so the full re-query stays fast, rather than trusting a stale cache.
- Apply always invalidates → a plan after an apply is never served stale.
- The cache is regenerable (digest-keyed); no atomic-fsync durability needed (per
  -pid temp + rename).
