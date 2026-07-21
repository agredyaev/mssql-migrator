# ADR-0003: Data-oriented arena workspace, lazy script bodies

Status: Accepted
Date: 2026-07-21

## Context

Plan phase holds every managed object of the catalog to diff repo vs live. Naive
object-graph (per-object `String` keys/paths, retained bodies) = many small heap
allocations + large resident set. Scan hot path (walk → parse → diff) is the
CLI-wall SLO target (< 150 ms). See also `docs/data-oriented-layout-policy.md`.

## Decision

Workspace is column-oriented (SoA) over a single interned string arena, not an
object graph.
- `StringArena` (`crates/core/src/domain/arena/`): one append-only `Arc<[u8]>`
  buffer + dedup map. Interns object keys, paths, git strings. `StrOff(off,len)`
  locates a slice.
- `ObjectEntry` (`crates/core/src/domain/object/mod.rs`): `checksum:[u8;32]`,
  `key_off:StrOff`, `script_id:u32`, ids/flags. ~48 bytes. No body field.
- Side columns in `WorkspaceCold`: script path offsets, checksums, prior
  checksums.

Script bodies are NOT retained. Scan reads each file, computes the SHA-256, and
drops the bytes immediately (`crates/core/src/scan/parse_object.rs:file_checksum`).
Apply re-reads the body from disk on demand, only for the apply set, and
re-verifies it against the scan-time checksum (`apply/script_read.rs::verified_body`,
`apply/transitions.rs`). Skip-unchanged objects never read their body after scan.

## Consequences

- Per-object retained memory = hundreds of bytes (entry + interned key/path +
  two checksums), independent of SQL body size (which may be KB–MB).
- Two-pass (checksum-all at scan, re-read bodies for the apply set at apply) is
  the architecture; no separate change needed to achieve lazy bodies.
- `compute_diff` for 100k objects = 56 ms in-memory (`crates/core-dev/tests/scale_footprint.rs`).
  Diff is never the bottleneck; DB round-trips are (see ADR-0011).
- Cost: arena is append-only/contiguous → selective omission of interned strings
  is impractical; irrelevant, since bodies are not interned. Arena caps at 4 GiB
  (u32 offsets), far above any real repository.
