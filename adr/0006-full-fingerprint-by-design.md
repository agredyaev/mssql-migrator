# ADR-0006: Full live-definition fingerprint; modify_date shortcut rejected

Status: Accepted
Date: 2026-07-21

## Context

ADR-0004 drift detection hashes each object's full live definition (columns,
constraints, FKs, indexes, module `OBJECT_DEFINITION`). Expensive. Tempting
shortcut: trust `sys.objects.modify_date` — skip the fingerprint for objects
whose modify_date did not advance past the last apply. Would make drift detection
constant-time without a DDL trigger (ADR-0005).

## Decision

Reject `modify_date` (and any single cheap catalog timestamp) as the drift
signal. Compute the full fingerprint for every object that must be checked.

Reason: `sys.objects.modify_date` does NOT bump on every definition change. Known
gaps: creating/dropping/altering an index on a table does not update the table's
`modify_date`; some ALTERs do not either. A modify_date-based skip would miss
real out-of-band drift → silent wrong plan → the exact hazard ADR-0004 exists to
prevent. Correctness of drift detection outranks its cost.

Order in the read path (`load_checksums_header.sql`): "no stored fingerprint"
(legacy/never-captured → no baseline → treat as drift) is checked BEFORE the
suspect fast-path, so the incremental optimization (ADR-0005) can never mask a
missing baseline.

## Consequences

- Drift guarantee is sound: any definition change to a managed object is caught,
  including index-only changes.
- The only sound way to make it faster is (a) fingerprint fewer objects via a
  trustworthy change signal — a DDL trigger, ADR-0005 — or (b) rewrite the
  fingerprint query set-based (one pass vs N correlated `OUTER APPLY`). (b) is a
  recorded future option; high-risk on a 286-line safety-critical asset shared by
  read and insert paths, so not done in the audit.
- ADR-0005's fail-safe (`@force_full` when trigger absent/disabled) preserves this
  full-fingerprint guarantee whenever the cheap signal is untrustworthy.
