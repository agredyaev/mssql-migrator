# Technical Document: Rust CLI (`rmig`, `rmigd`)

Lifecycle: `Current`.

## Purpose

Describe the **production operator entry points** for the Rust migrator: the `rmig` CLI and optional `rmigd` session daemon that reuse a warm SQL connection.

## Scope

- `rust/crates/cli/src/main.rs` — argument parsing, env load, command dispatch
- `rust/crates/rmigd/` — session daemon (feature `session-daemon` on `migrator-core`)
- Build outputs: `rust/target/release/rmig`, `rust/target/release/rmigd` (or `release-fast` profile for tests)

## System context

Operators invoke `rmig` with the same command names as the historical Go CLI: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`. Configuration uses `RM_*` environment variables loaded from `--env` (default `.env`). When `RMIG_SESSION` points at a `rmigd` Unix socket, `engine::run_command` connects through `session::connect_daemon` instead of opening a new TDS connection per invocation.

## Interfaces and boundaries

- Inputs: argv, dotenv file, `RM_*` / `RMIG_*` environment
- Outputs: stderr logs, stdout plan JSON when `--json`, exit codes from `migrator_core::error`
- Boundaries: CLI does not implement planning logic; all work delegates to `migrator_core::engine::run_command`

## Assumptions and constraints

- Assumption: `validate_config` passes before any SQL connect.
- Constraint: `version` does not load `.env` or connect to SQL.

## Nominal flow

1. Parse command and flags (`--env`, `--json`).
2. `build_config` + `validate_config`.
3. `run_command(Command, &cfg)` → timings + optional plan.
4. Write plan stdout / reports when configured (`export::write_reports`).

## Off-nominal behavior and failure containment

- Invalid command or config: exit before connect.
- Engine errors: non-zero exit; `Error::PlanBlocked` maps to exit code 10 on migrate.

## Verification and validation

- `cd rust && cargo build -p migrator-cli -p migrator-rmigd`
- `make rust-slo` (uses `rmigd` + warm plan)
- Integration: `rust/crates/core/tests/integration_plan.rs`

## Operations and recovery

- Release build: `make release-build` or `cargo build --release -p migrator-cli`.
- Session daemon: start `rmigd`, set `RMIG_SESSION` to socket path (see `module-cache-session.md`).

## Open issues and non-goals

- Non-goals: Windows service packaging for `rmigd`.

## References

- `docs/specs/rust/module-engine.md`
- `docs/specs/rust/module-config-export.md`
- `docs/rust-port-plan.md`
