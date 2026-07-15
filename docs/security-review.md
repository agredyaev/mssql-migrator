# Security Review

Lifecycle: `Current`.

## Purpose

Record the security model of `rmig`: how secrets are handled, how SQL injection
and path traversal are prevented, what logging is allowed, and which risks
remain. It answers: "Is it safe to run this in CI against a production catalog?"

## Scope

- Secret handling and redaction: `crates/core/src/config/cold.rs`, `crates/core/src/config/debug.rs`, `crates/core/src/session/auth.rs`.
- SQL safety: `crates/core/src/sql_ident.rs`, `crates/core/src/session/client.rs`, parameterized catalog queries in `crates/core/src/db`.
- Input validation: `crates/core/src/scan/walk.rs`, `crates/core/src/config/validate.rs`.
- Out of scope: SQL Server-side permissions and network security (operator-owned).

## System Context

`rmig` reads untrusted repository paths and contents and connects to SQL Server
with operator-supplied credentials. It runs in CI, where logs may be retained,
so it must never emit secrets and must never assemble SQL from untrusted values
without escaping.

## Interfaces And Boundaries

- Inputs: credentials and connection settings from environment variables; repository file paths and contents.
- Outputs: structured logs (no secrets), generated T-SQL, and error messages.
- Trust boundary: everything from the filesystem and config is untrusted at the SQL boundary; identifiers are bracket-escaped and catalog reads are parameterized.
- Ownership boundaries: redaction is owned by `crates/core/src/config/debug.rs` and `crates/core/src/config/cold.rs`; identifier safety by `crates/core/src/sql_ident.rs`.

## Assumptions And Constraints

- Assumptions: operators supply credentials via a secret store, not committed files.
- Assumption: repository `.sql` script bodies are TRUSTED input. The migrator executes them verbatim (wrapped only in a transaction) with the deploy credentials, so write access to the migrations repository is equivalent to arbitrary SQL execution against the target server. Only identifiers and catalog reads are treated as untrusted; script contents are not sandboxed.
- Assumption: `rmigd` token auth is OPT-IN. An empty configured token disables authentication entirely (`crates/core/src/session/auth.rs`); in that mode the Unix-socket file permissions (`0600` socket in a `0700` directory) are the only access control, so the socket path must never be placed in a group/world-accessible directory.
- Constraints:
  - Do not print secrets; do not log connection strings or credentials; do not include credentials in error messages.
  - `password` and `session_token` are redacted in all `Debug` output; `Config`, `ConfigCold`, and the session `Request` (`crates/core/src/session/protocol.rs`) implement redacting `Debug` by hand.
  - User-controlled schema/database identifiers are emitted only through `bracket_ident`, which validates the component and doubles `]`.
  - Catalog reads use parameterized queries (`@p1`/`@p2`/`@p3`), not string interpolation of untrusted values.
  - Dynamic `CREATE SCHEMA` is wrapped in `EXEC(...)` (required by T-SQL batch rules) with the bracketed identifier and the `SCHEMA_ID` probe both single-quote-escaped, so a schema name containing `'` cannot break out (`crates/core/src/apply/schemas.rs`).

## Nominal Flow

1. Config is built from the environment; `crates/core/src/config/validate.rs` rejects control characters in server and database names.
2. Secrets live in `ConfigCold` behind an `Arc`; any `Debug` rendering shows `<redacted>`/`<unset>` (`crates/core/src/config/debug.rs`, `crates/core/src/config/cold.rs`).
3. Schema/database names are quoted via `bracket_ident` before use (for example `USE [db]` in `crates/core/src/session/client.rs`).
4. The `rmigd` session token is compared in constant time (`crates/core/src/session/auth.rs`).

## Off-Nominal Behavior And Failure Containment

- Failure mode: a malicious schema/database name contains `]` or path-traversal characters.
  Containment: `bracket_ident` validates the component (rejecting `.`, `..`, `/`, `\`, NUL) and doubles `]`, preventing injection (`crates/core/src/sql_ident.rs`).
- Failure mode: a non-UTF-8 or traversal path appears in the tree.
  Containment: the scan rejects it rather than performing a lossy conversion (`crates/core/src/scan/walk.rs`).
- Failure mode: code accidentally debug-prints config.
  Containment: the hand-written `Debug` impls redact `password` and `session_token`, so no secret reaches the log.
- Failure mode: an attacker probes the daemon token.
  Containment: constant-time comparison avoids leaking matched-prefix length via timing.
- Failure mode: a catalog database directory has an illegal or over-long name.
  Containment: `crates/core/src/config/ensure_db.rs` validates every name through `bracket_ident` before opening any connection, failing with exit `8` instead of emitting an invalid `CREATE DATABASE` statement.
- Failure mode: a schema name contains a single quote (legal inside brackets, e.g. `O'Brien`).
  Containment: `build_create_schema_sql` doubles `'` for both the `EXEC` string and the `SCHEMA_ID` literal, so the value stays data, not SQL (`crates/core/src/apply/schemas.rs`).

## Verification And Validation

- Contracts and checks: `crates/core/src/config/cold.rs` tests (redaction), `crates/core/src/tests/protocol_test.rs` (session `Request` token redaction), `crates/core/src/session/auth.rs` tests (constant-time compare), `crates/core/src/tests/sql_ident_test.rs` (escaping and length limit), `crates/core/src/tests/ensure_db_test.rs` (catalog-name validation), `crates/core/src/tests/error_test.rs` (exit-code mapping), `crates/core/src/tests/schema_sql_test.rs` (idempotent, injection-safe `CREATE SCHEMA`), `crates/core/src/tests/proxy_test.rs` (socket limits).
- Evidence artifacts: test output and CI logs that contain `<redacted>` rather than secrets.
- Exit criteria: no secret appears in any log or error; no untrusted value reaches SQL unescaped.

## Operations And Recovery

- Routine operation: supply credentials via CI secrets; never commit `.env` with real passwords (the loader warns on world-readable `.env`).
- Recovery or rollback: if a secret is suspected leaked, rotate the SQL login and `RMIG_SESSION_TOKEN`; redaction prevents recurrence in logs.

## Open Issues And Non-Goals

- Open issues: the constant-time token check intentionally short-circuits on length difference (token length is not treated as sensitive); the daemon transport is a local Unix socket, not an encrypted channel.
- Accepted (deferred) internal risks, reviewed and judged safe:
  - The string arena (`crates/core/src/domain/arena/builder.rs`, `crates/core/src/domain/arena/slice.rs`, `crates/core/src/domain/shared/mod.rs`) uses unchecked slice access and `panic!` for offset lookups. Offsets are constructed only from scan input already validated as UTF-8, so untrusted repository data cannot reach them; bounds checks would add cost to a hot path. Left unchanged.
  - `crates/core/src/domain/arena/builder.rs` casts buffer length to `u32`, capping a single arena at 4 GiB, far above any real migration repository.
  - Driver error text from `crates/core/src/driver/mssql_query.rs` is surfaced as-is. TDS error messages do not carry credentials, so they are passed through for diagnostics.
  - The `relaxed_cache_count` interpolation in `crates/core/src/db/batch.rs` substitutes only a `usize`, so it cannot inject SQL.
- Non-goals: this document does not cover SQL Server-side authorization, TLS configuration, or host hardening.

## References

- Canonical source paths: `crates/core/src/config/debug.rs`, `crates/core/src/config/cold.rs`, `crates/core/src/sql_ident.rs`, `crates/core/src/session/auth.rs`.
- Related contracts and scripts: `docs/operational-contract.md`, `docs/ci-usage.md`.
