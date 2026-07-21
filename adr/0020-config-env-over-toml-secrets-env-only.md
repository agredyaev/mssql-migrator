# ADR-0020: Config — env over TOML, secrets env-only, fail-fast validation

Status: Accepted
Date: 2026-07-21

## Context

Config has non-secret settings (server, port, roots, timeouts, flags) and secrets
(DB user/password, daemon session token). CI supplies secrets from a secret store
as process variables. Committing secrets to a config file, or silently coercing a
malformed setting, are the hazards.

## Decision

- Precedence: process env OVER typed TOML (`config/env_build.rs`:
  `std::env::var(name).ok().or(toml_value)`). Default file `config.toml`.
- Secrets are env-only, never from TOML: `RM_DB_USER`, `RM_DB_PASSWORD`,
  `RMIG_SESSION_TOKEN` read via bare `std::env::var`. `validate_config` rejects a
  run whose secrets are missing and states they are not read from `config.toml`.
- Fail-fast validation before connect: boolean env vars with an unrecognized
  value → `Error::Config` (exit 2), not coerced (ADR-0010); control chars in
  server/db names rejected; port 1–65535.
- Redaction: hand-written `Debug` on `Config`/`ConfigCold` masks
  `password`/`session_token` (`config/debug.rs`, `config/cold.rs`); no `tracing!`
  logs a secret. Daemon token compared constant-time (`session/auth.rs`).

## Consequences

- Secrets never live in a committed file; a leaked `config.toml` exposes no
  credentials.
- Invalid config fails before any DB contact with a classified code, never
  silently uses a dangerous default.
- Debug/backtrace/log output cannot leak a secret (empirically checked in the
  audit failure matrix — wrong password never appears in stderr).
