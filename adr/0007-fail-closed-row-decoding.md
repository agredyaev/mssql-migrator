# ADR-0007: Result-column decoding fails closed on unknown types

Status: Accepted
Date: 2026-07-21

## Context

`from_tiberius` (`crates/core/src/driver/row.rs`) converts a `tiberius::Row` to
the tool's `RowData`. It tries a fixed ladder of column types (`&[u8]`, `&str`,
`bool`, integers). Original behavior: if no rung matched, the cell became
`Cell::Null`. So a genuine SQL `NULL` and an unhandled/unexpected column type were
indistinguishable — both `Null`. A catalog query column whose type the ladder
does not cover would silently read as `Null` → wrong plan, no error signal.

## Decision

Fail closed. `from_tiberius` returns `Result<RowData>`. A column is `Cell::Null`
only when some `try_get` returned `Ok(None)` (a real NULL of a supported type).
If every typed attempt errored (type not understood), return
`Error::Sql("unsupported column type … for column … at index …")` (exit `5`,
`EXIT_SQL`). No new error variant. Callers propagate
(`driver/db_client.rs`, `session/daemon_rpc.rs`).

Ladder includes `i16` (smallint): `COL_LENGTH()` and `sys.columns.max_length`
return smallint, which the old ladder silently nulled — a latent bug (the
`column_exists` probe returned false for existing columns), fixed here.

## Consequences

- A future catalog query column of an unhandled type is a hard, located error,
  not a silent NULL that corrupts the plan.
- All current queries project `nvarchar/varchar/varbinary/bit/tinyint/smallint/
  int/bigint` — nothing existing breaks; the DB regression suite is the check
  (no unit test practical: `tiberius::Row` has no public constructor).
- Commit `2246b71`.
