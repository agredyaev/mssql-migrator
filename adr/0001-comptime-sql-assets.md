# ADR-0001: Comptime SQL assets, no inline SQL

Status: Accepted
Date: 2026-07-21

## Context

Tool builds and runs T-SQL against SQL Server. SQL scattered as inline string
literals across Rust = hard to review, hard to audit for injection, easy to
drift from what runs. Migration tool touches production catalogs; SQL surface
must be reviewable in one place.

## Decision

All runtime queries live in `.sql` files under repo-root `sql/`. Bound at
compile time via `include_str!` into `crate::sql::*` consts
(`crates/core/src/sql/mod.rs`). Rust code never assembles query text from ad-hoc
literals. Composite queries built by `concat!` of fragment files (example:
`LOAD_CHECKSUMS` = header + shared `_object_canonical_state.sql`).

Enforced by gate `scripts/check-no-inline-sql.sh` (via `make arch`): any SQL
keyword in `crates/core/src` non-test code fails unless `include_str!` or marker
`// sql-gate:allow`. Test code exempt.

Dynamic identifiers never interpolated raw. `bracket_ident`
(`crates/core/src/sql_ident.rs`) validates (reject `. / \ NUL`, max 128 chars)
and doubles `]` → `]]`. Data values bind as `@pN` params via `OPENJSON`, not
string-substituted.

## Consequences

- SQL reviewable as one asset tree. Injection surface = `bracket_ident` sites +
  the `@pN` bindings, both small and audited.
- Every query change is a diff in `sql/`, visible to reviewers.
- Cost: fragment composition (`concat!`) and `@pN` renumbering by text
  `.replace` in `db/catalog.rs` are brittle; fenced by unit tests asserting
  placeholder positions + the DB regression suite.
- Zero-length or template `.sql` files exist (`create_database.sql` etc. are
  `format!` templates filled with escaped idents) — not truncation, by design.
