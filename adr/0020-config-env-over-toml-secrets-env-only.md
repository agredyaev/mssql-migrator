# ADR-0020: Config — peer identity and secrets are environment-only

Status: Accepted
Date: 2026-07-21
Updated: 2026-07-24

## Context

`rmig` combines repository TOML with SQL credentials and the daemon token from
the process environment. If repository TOML can select the SQL endpoint, TLS
policy, or daemon socket, it can redirect those environment secrets to another
peer. Filesystem paths and execution flags do not identify a credential peer.
Implicit name-only adoption is also a privileged operator decision: repository
TOML must not enable it.

## Decision

- TOML accepts only `[paths]` and `[execution]`.
- Peer identity and transport policy are process-environment only:
  `RM_DB_SERVER`, `RM_DB_PORT`, `RM_DB_ENCRYPT`,
  `RM_DB_TRUST_SERVER_CERTIFICATE`, and `RMIG_SESSION`.
- SQL authentication is the only supported mode. `RM_DB_USER` and
  `RM_DB_PASSWORD` are always required; no authentication-mode selector exists.
- Secrets remain process-environment only: `RM_DB_USER`, `RM_DB_PASSWORD`, and
  `RMIG_SESSION_TOKEN`.
- `RMIG_ALLOW_ADOPT` is process-environment only. TOML
  `[execution].allow_adopt` is rejected.
- `load_toml_config` rejects every environment-only TOML key and names its
  replacement variable. TOML parse errors omit source excerpts and values.
- Fail-fast validation before connect: boolean env vars with an unrecognized
  value → `Error::Config` (exit 2), not coerced (ADR-0010); control chars in
  server/db names rejected; port 1–65535.
- Redaction: hand-written `Debug` on `Config` masks `password` and
  `session_token` (`config/debug.rs`); no `tracing!` logs a secret. Daemon token
  is compared constant-time (`session/auth.rs`).

## Consequences

- Repository configuration cannot select the peer that receives SQL credentials
  or `RMIG_SESSION_TOKEN`.
- Existing TOML `[database]` and `[session]` keys fail closed. Operators move
  those values to the named process variables.
- Existing TOML `[execution].allow_adopt` fails closed. An operator must set
  `RMIG_ALLOW_ADOPT=1` for implicit adoption during `migrate` or run
  `rmig baseline` explicitly.
- Invalid config fails before any DB contact with a classified code, never
  silently uses a dangerous default.
- Debug, parser, and log output do not include secret values.

## Verification

- `crates/core/src/tests/toml_config_test.rs`
- `crates/core/src/tests/env_build_test.rs`
- `cargo test -p migrator-core --lib config::`
