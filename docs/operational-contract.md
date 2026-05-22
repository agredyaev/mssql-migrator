# Operational contract

Lifecycle: `Current`.

## Purpose

How **`rmig`** is built, configured, and operated in this repository.

## Build

```bash
make build           # target/release/rmig, rmigd (debug symbols for profiling)
make release-build   # bin/rmig (--profile release-dist: fat LTO, strip, codegen-units=1, panic=abort)
make check           # arch, fmt, clippy, unit tests
```

## CLI

- **Invocation:** `rmig [--env <path>] [--json] <command>` (bare `rmig` or `rmig --help` prints usage)
- **Commands:** `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`
- **Entry:** `crates/cli/src/main.rs` → `migrator_core::engine::run_command`

### Required environment

| Variable | Meaning |
|----------|---------|
| `RM_DB_SERVER` | SQL Server host |
| `RM_DB_DATABASE` | Target database |
| `RM_SQL_ROOT` | Repo SQL tree root |
| `RM_SQL_BASE` | Base path for migrations / scaffold (often same as `RM_SQL_ROOT`) |

Loaded from `--env` file and process environment (`config::build_config`).

### Common optional variables

| Variable | Effect |
|----------|--------|
| `RM_SKIP_GIT=1` | Skip git preload; full catalog inspect |
| `RM_REPORT_DIR` | Write `.plan.json` / `.report.json` |
| `RMIG_SESSION` | Unix socket to `rmigd` (warm connection) |
| `RMIG_CATALOG_CACHE=0` | Disable DB catalog cache |
| `RMIG_GATE_SKIP_DB_RESET=1` | Integration: skip DROP/CREATE |

Full list: [`rmig-rust.md`](rmig-rust.md), [`specs/rust/module-config-export.md`](specs/rust/module-config-export.md).

## Artifacts

When `RM_REPORT_DIR` is set, compact JSON plan and run report are written on successful plan/migrate completion (`export::report`).

## Integration verification

```bash
make db-up
make e2e-all
make prod-gate
make slo
```

## References

- [`solution.md`](solution.md)
- [`runbook.md`](runbook.md)
- [`specs/README.md`](specs/README.md)
