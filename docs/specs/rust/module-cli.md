# Rust CLI (`rmig`, `rmigd`)

Lifecycle: `Current`.

## Purpose

Describe the **production operator entry points** for the Rust migrator: the `rmig` CLI and optional `rmigd` session daemon that reuse a warm SQL connection.

## Scope

- `crates/cli/src/main.rs` - argument parsing, env load, command dispatch
- `crates/rmigd/` - session daemon (feature `session-daemon` on `migrator-core`)
- Build outputs: `target/release/rmig`, `target/release/rmigd`, `bin/rmig` via `make release-build` (`--profile release-dist`; tests may use `release-fast` profile)

## System context

Operators invoke `rmig` with commands: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, `version`. Configuration uses `RM_*` environment variables loaded from `--env` (default `.env`, optional when missing) or from an explicit `--env <path>` (required; missing or unreadable file fails before connect). When `RMIG_SESSION` points at a `rmigd` Unix socket, `engine::run_command` connects through `session::connect_daemon` instead of opening a new TDS connection per invocation.

## Interfaces and boundaries

- Inputs: argv, dotenv file, `RM_*` / `RMIG_*` environment
- Outputs: stderr logs and timings JSON, stdout plan JSON when `plan --json`, exit codes from `migrator_core::error`
- Boundaries: CLI does not implement planning logic; all work delegates to `migrator_core::engine::run_command`

## Assumptions and constraints

- Assumption: `validate_config` passes before any SQL connect.
- Constraint: `version` does not load `.env` or connect to SQL.

## Nominal flow

1. Parse command and flags (`--env`, `--json`).
2. `build_config` + `validate_config`.
3. `run_command(Command, &cfg)` → timings + optional plan.
4. For `plan --json`, write plan JSON to stdout and timings JSON to stderr; write reports when configured (`export::write_reports`).

## Off-nominal behavior and failure containment

- Invalid command or config: exit before connect.
- Engine errors: non-zero exit; `Error::PlanBlocked` maps to exit code 10 on migrate.

## Verification and validation

- `cargo build -p rmig -p rmigd`
- `make slo` (uses `rmigd` + warm plan)
- Integration: `crates/core/tests/integration_plan.rs`
- CLI arg/flag parsing: `crates/cli/src/tests/args_test.rs`
- Logging setup: `crates/cli/src/tests/logging_test.rs`
- Plan JSON roundtrip: `crates/core/tests/plan_json_roundtrip_test.rs`

## Operations and recovery

- Release build: `make release-build` (`cargo build --profile release-dist -p rmig`) or `cargo build --release -p rmig` for profiling binaries.
- Session daemon: start `rmigd`, set `RMIG_SESSION` to socket path (see `module-cache-session.md`).

## Open issues and non-goals

- Non-goals: Windows service packaging for `rmigd`.

## References

- `docs/specs/rust/module-engine.md`
- `docs/specs/rust/module-config-export.md`
- `docs/rmig-rust.md`
