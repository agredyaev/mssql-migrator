# ADR-0010: TLS on by default; `encrypt=false` disables all TDS TLS

Status: Accepted
Date: 2026-07-21

## Context

Migrator connects to SQL Server with operator credentials, runs in CI where logs
persist. Login packet and traffic must be encrypted by default. Local Docker
fixtures have no server certificate and need a plaintext opt-out. tiberius maps
encryption to `EncryptionLevel::{Required, Off, NotSupported}`.

## Decision

Secure by default (commit `09ccefa`): `Config::default()` sets encrypt ON, trust
server certificate OFF (`config/default.rs`). Applied as
`EncryptionLevel::Required` + cert validation (`driver/mssql.rs`).

Boolean env vars validated fail-closed: an unrecognized value for `RM_DB_ENCRYPT`
/ `RM_DB_TRUST_SERVER_CERTIFICATE` (and 5 more) returns `Error::Config` (exit `2`)
before connect — `RM_DB_ENCRYPT=ture` is rejected, never coerced to false
(`config/env_parse.rs::validate_boolean_envs`).

`encrypt=false` maps to `EncryptionLevel::NotSupported` (no TLS at all, including
the login packet), not `Off` (which would TLS-protect login only). Chosen because
`false` is an explicit local-fixture opt-out; the default is `true`.

## Consequences

- Production default: login + traffic encrypted, cert validated.
- `encrypt=false` is a sharp edge: with `NotSupported` the LOGIN7 password crosses
  the wire with only TDS XOR scrambling. tiberius logs a loud
  "TLS encryption is not enabled" warning. Operators setting `false` accept plain
  login.
- Recorded future option (audit M2): flip `false` → `Off` for login-only TLS.
  Not done — changes opt-out semantics against unknown fleet servers, and local
  TLS-less fixtures break under `Off`. Revisit per-environment.
- Secrets are env-only, never from TOML, and redacted in every `Debug` (ADR
  context: `config/cold.rs`, `config/debug.rs`). Daemon token compared in
  constant time (`session/auth.rs`).
