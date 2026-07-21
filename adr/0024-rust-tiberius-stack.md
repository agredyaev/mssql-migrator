# ADR-0024: Rust + tiberius stack; feature-gated daemon; benches isolated

Status: Accepted
Date: 2026-07-21

## Context

Port from the prior implementation to a language/stack that gives a small,
statically-linked, fast CLI with no runtime dependency, precise error handling,
and a pure-Rust SQL Server driver (no ODBC/system driver install in CI). Branch:
`refactor/port-to-rust`.

## Decision

- **Rust**, edition 2021, MSRV pinned to the CI toolchain (1.96). Two shipped
  binaries: `rmig` (CLI), `rmigd` (daemon). Panic=abort in `release-dist`.
- **tiberius 0.12.3** as the MSSQL driver (`default-features = false`, features
  `tds73, chrono, native-tls`). Pure Rust TDS — no ODBC, no sqlx, no system driver
  to install. Async on **tokio** (multi-thread).
- **No ORM / query builder**: queries are hand-authored SQL assets (ADR-0001).
- **Workspace layout**: `migrator-core` (lib, all logic), `rmig`, `rmigd`,
  `migrator-core-dev` (benches/pprof/dhat, `publish=false`). `default-members` =
  the two shipped binaries.
- **Daemon feature-gated** (`session-daemon`): the whole daemon module compiles
  only with the feature. **Bench crate isolated**: `check-rust-release-deps.sh`
  fails if pprof/criterion/dhat/`migrator-core-dev` appear in a shipped binary's
  dependency tree.

## Consequences

- CI/prod need no database client install; the binary is self-contained.
- Dev/bench tooling (criterion/pprof/dhat) can never leak into a release artifact
  — enforced, not hoped.
- tiberius shapes some choices: its result API materializes rows (ADR-0011), it
  has no good TVP support (hence `OPENJSON` for key sets), and its type-decode
  surface is why decoding fails closed (ADR-0007).
- Supply chain gated by `cargo deny` (advisories/bans/licenses/sources).
