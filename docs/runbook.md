# Runbook

Lifecycle: `Current`.

## Purpose

Operator steps after a failed **`rmig`** run: what to read, safe re-run, and common exit codes.

## Before re-run

1. Read stderr (human logs) or `--json` structured logs.
2. If `RM_REPORT_DIR` is set, inspect `.plan.json` and `.report.json`.
3. Confirm `RM_DB_*` and `RM_SQL_ROOT` point at the intended server and SQL tree.

## Exit codes (common)

| Code | Meaning |
|------|---------|
| 0 | Success |
| 10 | Plan blocked (DDL change needs transition migration) |
| 7 | Session lock held by another process |

Full map: `crates/core/src/error.rs` (`exit_code`).

## Blocked migrate

When `migrate` exits **10**, inspect `_migrations/` scaffold files under the affected table path. Commit a real migration SQL file or revert the layout change before retrying.

## Prod gate failure

Run `make prod-gate` locally with the same `RM_SQL_ROOT` and baseline under `crates/core/tests/testdata/prod_gate/`. See [`prod-gate.md`](prod-gate.md).

## Integration / CI

```bash
make db-up
make e2e-all
```

## References

- [`operational-contract.md`](operational-contract.md)
- [`solution.md`](solution.md)
- [`specs/rust/module-engine.md`](specs/rust/module-engine.md)
