# ADR-0005: Incremental drift detection via DDL-trigger versioning

Status: Accepted
Date: 2026-07-21
Supersedes part of: ADR-0004 (adds the incremental path; blocking semantics unchanged)

## Context

ADR-0004 fingerprints every managed object's live definition on every plan. Cost
is O(catalog size): the fingerprint query (`sql/audit/_object_canonical_state.sql`)
ran a per-object correlated `OUTER APPLY` (OBJECT_DEFINITION / column+constraint
+index enumeration → JSON → HASHBYTES). Measured: ~73% of plan wall, ~27 s at 20k
objects, minutes at 100k — paid every plan regardless of how few objects changed
(`keys_json` = full workspace; git-delta scopes catalog inspection, not the
checksum/drift query). See ADR-0006 for why the fingerprint itself is not cheaper.

## Decision

Make the fingerprint O(objects changed since their last apply), not O(catalog).

- `bootstrap_tables.sql`: monotonic `azdo_deploy_meta.ddl_seq` sequence, an
  `object_ddl(schema_name, object_name, ddl_version)` table (one version per
  object), and `history.applied_ddl_version`.
- `bootstrap_drift.sql`: a best-effort database DDL trigger
  `azdo_deploy_meta_ddl_watch` stamps `object_ddl` with the next sequence value on
  every CREATE/ALTER/DROP of a managed kind (indexes included). Names lowercased
  to match normalized keys; own metadata schema excluded. Requires
  `QUOTED_IDENTIFIER ON` at CREATE (XML `EVENTDATA()` methods) — set in the asset.
- Apply records the observed version into `history.applied_ddl_version` (read
  in-SQL from `object_ddl` at insert time — no Rust API change).
- Read path (`load_checksums_header.sql`) fingerprints only "suspects": objects
  where `object_ddl.ddl_version > applied_ddl_version`, or with no recorded
  version, or with no stored fingerprint. Non-suspects report drift = 0 without
  the fingerprint. The suspect gate uses a `WHERE rows.is_suspect = 1` filter on
  a `(VALUES(1))` derived table inside the shared canonical-state block — a `CASE`
  guard does NOT stop the optimizer from evaluating the subqueries; the WHERE
  filters the row before projection.

Fail-safe: if the trigger is absent or disabled, `@force_full = 1` (checked
in-SQL against `sys.triggers`) forces every object to a suspect → full
fingerprint = prior behavior. Drift is never missed. Insert path keeps
`is_suspect = 1` for all rows, so it always captures the baseline.

## Consequences

- Fingerprint is O(suspects). Measured (no drift, warm): 500 tables checksum
  query 1330 ms → 36 ms; 2000 tables 2100 ms → 120 ms; 5k git-delta re-plan full
  plan **710 ms** (checksum 5957 ms → 275 ms, 21×).
- Stored fingerprint, output columns, and blocking semantics (ADR-0004)
  unchanged. Only which objects get re-fingerprinted differs.
- Needs one permission: create a database DDL trigger. Without it, degrades to
  the full scan — never breaks. Server-side footprint: one sequence, one table,
  one trigger, one history column.
- Standalone indexes tracked as their own objects (index events stamp the index,
  not the table — a table fingerprint does not include non-key indexes).
- Covered by `crates/core/tests/incremental_drift_test.rs` (tracking installed,
  no false positive, disabled-trigger fallback still blocks) + the unchanged
  `drift_e2e_integration.rs` suite. Commit `50d51c0`/`241d20f`.
- Not addressed here: the checksum LOAD is still O(catalog) (loads all stored
  checksums to classify skip vs changed) — cheap per row, the fingerprint was the
  cost. See ADR-0011.
