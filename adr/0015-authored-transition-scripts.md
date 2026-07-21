# ADR-0015: Table changes via authored transition scripts, not generated ALTERs

Status: Accepted
Date: 2026-07-21

## Context

Two ways to migrate a changed table: (a) the tool diffs old vs new shape and
synthesizes ALTER DDL; (b) the author writes an explicit migration script. Auto
-synthesized ALTERs are unsafe for tables — column drops/type changes/data moves
have no single correct DDL, and a generated ALTER can silently lose data.

## Decision

Tables migrate via **author-written transition scripts**, not generated DDL. The
tool executes repo-authored transition `.sql`, it does not synthesize table DDL.

- Transition path:
  `<database>/<schema>/tables/_migrations/<table>/<ordinal>_<commit>_<slug>.sql`.
  `<ordinal>` exactly 3 digits, `<commit>` ≥7 hex, `<slug>` non-empty. Ordinals
  give a deterministic apply order.
- Modules (view/proc/function/trigger) DO apply generated-free via
  `CREATE OR ALTER` — idempotent, no data risk.
- A changed table with no transition script that covers it → blocked (ADR-0004,
  exit 10). The `scaffold` step writes skeleton transition files for blocked
  objects to help the author, but never applies them.
- Fresh tables record their historical transitions as already-applied so later
  runs don't replay them (`crates/core/src/apply/table_records.rs`).
- Transition apply advances the table's object baseline in the SAME transaction
  as the final transition (avoids perpetual reprocess).

## Consequences

- No tool-generated table DDL can lose data; every table change is human-authored,
  reviewed, ordered, and checksum-verified before apply (ADR-0002).
- Column/constraint/index changes to a table require an explicit transition; this
  is the deliberate friction that makes destructive changes visible.
- Modules stay ergonomic (`CREATE OR ALTER`) since re-emitting a module is safe.
