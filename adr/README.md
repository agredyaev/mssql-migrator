# Architecture Decision Records

Record load-bearing decisions for `rmig` (CLI) and `rmigd` (session daemon):
git-driven MSSQL schema/DDL migrator. One file per decision. Immutable once
`Accepted`; supersede with new ADR, do not rewrite history.

Location note: `adr/` sits outside `docs/`, so the NASA-11-section doc gates
(`ops/quality/scripts/check_doc_*.py`, scan roots `README.md`/`AGENTS.md`/`docs/**`)
do not apply. ADRs keep the standard Status/Context/Decision/Consequences shape.

Language: terse. Drop filler. Keep every technical fact and exact error string.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-comptime-sql-assets.md) | Comptime SQL assets, no inline SQL | Accepted |
| [0002](0002-audit-history-state-store.md) | Audit history is the state store; body+history commit atomically | Accepted |
| [0003](0003-data-oriented-arena-workspace.md) | Data-oriented arena workspace, lazy script bodies | Accepted |
| [0004](0004-structural-drift-fail-closed.md) | Structural live drift blocks the plan (fail-closed) | Accepted |
| [0005](0005-incremental-drift-ddl-trigger.md) | Incremental drift detection via DDL-trigger versioning | Accepted |
| [0006](0006-full-fingerprint-by-design.md) | Full live-definition fingerprint; modify_date shortcut rejected | Accepted |
| [0007](0007-fail-closed-row-decoding.md) | Result-column decoding fails closed on unknown types | Accepted |
| [0008](0008-rmigd-single-warm-connection.md) | rmigd holds one warm TDS connection, no pool | Accepted |
| [0009](0009-rmigd-socket-metrics.md) | rmigd metrics/health over the socket, not HTTP/Prometheus | Accepted |
| [0010](0010-tls-encrypted-by-default.md) | TLS on by default; `encrypt=false` disables all TDS TLS | Accepted |
| [0011](0011-plan-memory-o-catalog.md) | Plan cost and memory are O(catalog size) by construction | Accepted |
| [0012](0012-graceful-shutdown-via-drop.md) | Graceful shutdown drops the run future; server rolls back | Accepted |

## Verification evidence

Perf claims here come from the audit in
[`PRODUCTION-READINESS-AUDIT.md`](../PRODUCTION-READINESS-AUDIT.md) (§10, §10.1).
