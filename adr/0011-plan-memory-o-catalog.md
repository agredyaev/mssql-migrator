# ADR-0011: Plan cost and memory are O(catalog size) by construction

Status: Accepted
Date: 2026-07-21

## Context

Ask: make a plan not grow with catalog size — sub-second, flat memory, regardless
of how few objects changed. Investigated whether the residual O(catalog) cost
(after ADR-0005 removed the drift-fingerprint blowup) is reducible.

## Decision

Accept that a full plan is O(catalog size). Do not scope-shrink the scan or the
checksum load under the current output contract.

Measured breakdown of what remains (Tier2, git-delta, warm):
- Scan: reads + SHA-256 of every `.sql` file. CPU profile shows it is pure disk
  IO (`std::fs::read`), not hashing. ~175 ms at 5k, ~21 s cold at 100k.
- Checksum load: loads every stored checksum to classify skip vs changed. O(N)
  but cheap per row (~275 ms at 5k).
- `keys_json` (all workspace keys, ~2.5 MB at 100k = 0.46% of the 539 MB plan
  RSS) — not the memory driver.

Why not reducible without changing behavior:
- The plan output lists every managed object with its action; tests assert the
  full list. Emitting N actions requires N object identities → O(N) irreducible.
- Bodies are already lazy (ADR-0003); there is no per-object body memory to
  reclaim. The 136 MB in-memory arena is per-object metadata (keys/paths/
  checksums), inherent to holding N objects.
- To skip an unchanged object's checksum you must first know it is unchanged,
  which needs its checksum — chicken/egg. The L1 cache short-circuit is gated to
  empty workspaces only (`requires_live_state`), because a stale cache cannot be
  trusted for drift (ADR-0004); ADR-0005 makes drift cheap, not the classification.

Removing `keys_json` (server already has the keys) saves 0.46% and risks the
brittle `@pN` batch renumbering (ADR-0001) — not worth it.

## Consequences

- Real product scale (hundreds–low thousands of objects): plan is sub-second.
- Extreme scale (100k, unrealistic for a reporting layer): plan is single-digit
  to low-tens of seconds, dominated by scan disk IO + checksum load — not the old
  minutes-long drift fingerprint.
- Truly O(changes) memory/time would require scanning + planning only changed
  files (a scan-phase rewrite, git-blob-hash checksum cache) — a large change with
  stale-cache correctness risk, for a scale the product does not hit. Recorded, not
  built. Reproduce the curve with `crates/core-dev/tests/scale_footprint.rs`.
