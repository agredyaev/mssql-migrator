# Technical Document: DOD minimum-size execution roadmap

Lifecycle: `Current`.

## Purpose

Track **implementation** of minimum in-memory layout for the Rust plan pipeline (scan → diff → plan). Rules live in [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md); measurement in [`perf-footprint-audit.md`](perf-footprint-audit.md). This file is the **execution roadmap** with step IDs, status, and anti-drift rules.

## Scope

- Rust hot path: [`rust/crates/core/src/domain/`](../rust/crates/core/src/domain/), [`plan/`](../rust/crates/core/src/plan/), [`export/`](../rust/crates/core/src/export/)
- Verification: [`ops/perf/`](../ops/perf/), [`internal/app/testdata/perf/rust_footprint_baseline.json`](../internal/app/testdata/perf/rust_footprint_baseline.json)

**Out of scope:** Go code changes (unless parity task opened); CI perf gates.

## Kelley technique codes

| Code | Technique |
|------|-----------|
| **IDX** | String/pointer → `u32` index |
| **DER** | Derive at read/export, do not store |
| **SOA** | Struct-of-arrays / columnar |
| **SPARSE** | `HashMap<u32, T>` for rare fields |
| **ARENA** | Single byte buffer + offsets |
| **SLAB** | One pre-sized domain per run |
| **TAG** | `u8` enum + match |
| **OOB** | Skip unchanged rows / bit flags |
| **COLD** | Fat data off hot row |
| **VIEW** | Fat wire shape only at JSON/export |

## Baseline snapshot (2026-05-21, darwin/arm64 committed JSON)

| Type | `size_of` (B) | N×5k heap (struct bodies only) |
|------|--------------:|-------------------------------:|
| `Config` | 360 | — |
| `WorkspaceCold` | 688 | `Box` on heap; maps/arena/scripts |
| `Workspace` (hot) | 88 | `object_entries`, `object_keys`, `object_rows`, `cold` |
| `MigrationPlan` | 304 | + `Vec` heaps |
| `PlannedObject` | 144 | ~720 KB |
| `ObjectEntry` + `ObjectRow` | 56 + 6 | ~310 KB (`key_off` + optional `staging_key`) |
| `ScriptRow` | 40 | M-dependent |
| Diff loop dhat | — | **0 B/iter** (skip-heavy 5k) |

## Invariants (no drift)

1. Diff iterates `for i in 0..object_count` by row index (**CASE-1**).
2. Warmed skip-heavy 5k: dhat loop phase **0 B/iter** (regression = blocker).
3. Plan JSON wire unchanged: `make go-rust-e2e` (or documented subset) green after layout PRs.
4. No full-scan `HashMap` iteration in diff (**DOD-X2**).
5. No `SharedStr::new` in per-object diff loop after finalize.
6. Every layout PR updates **Status** below for touched step IDs.

## Status tracker

| ID | Phase | Kelley | Status |
|----|-------|--------|--------|
| 0.1 | Bootstrap | — | done |
| 0.2 | Policy link | — | done |
| 0.3 | Baseline in doc | — | done |
| verify | dhat 0 B/iter + unit tests | — | done |
| verify | `make go-rust-e2e` | — | env-dependent (pipeline materialize fix in `tests/common/pipeline.rs`) |
| P1 | Plan row slab | SLAB, TAG | done |
| P2 | Fill / skip | OOB, TAG | done |
| P3 | JSON VIEW | VIEW, DER | done |
| P4 | Git sparse | SPARSE, ARENA | done |
| P5 | Transitions sparse | SPARSE, IDX | done |
| P6 | Callers | VIEW | done |
| A1 | `StrOff` type | ARENA, IDX | done |
| A2 | `LayoutArena` on `Workspace` | ARENA | done |
| A3 | `SharedStr` VIEW boundary | VIEW | done |
| L1 | `key_off` + `object_keys` | IDX, ARENA | done |
| L2 | `db_id` table | IDX | done |
| L3 | `ObjectKey` VIEW for maps | VIEW | done |
| S1–S4 | `ScriptRow` | SOA, ARENA, DER | done |
| W1 | `object_path_cache` `StrOff` | ARENA, IDX | done |
| W2 | `transition_path_cache` `StrOff` | ARENA, IDX | done |
| W3 | Sparse maps `u32` row keys | IDX, SPARSE | done |
| W4 | `Workspace` hot shell + `Box<WorkspaceCold>` | COLD, SLAB | done |
| W5 | `object_rows` / `object_keys` on hot; `key_index` in cold | SOA, CASE-1 | done |
| W6 | `Deref`/`DerefMut` → cold (plan/scan API) | — | done |
| C1 | `Config` split | COLD | done |

## Per-struct unload matrix

### `PlannedObject` (hot) → `PlanRow` + VIEW

| Field today | Kelley | Target |
|-------------|--------|--------|
| 7× `SharedStr` | DER, VIEW | materialize from `Workspace` index `i` |
| `planned_action` | TAG | `PlanRow.action: u8` |
| `exists` | OOB | `PlanRow.flags` |
| `checksum` | SOA | `PlanRow.checksum` |
| `git` | SPARSE | `MigrationPlan.plan_git: HashMap<u32, PlannedGit>` |
| `transition_paths` | SPARSE | `MigrationPlan.plan_transitions: HashMap<u32, Vec<SharedStr>>` |

**Steps:** P1–P6. **Target row:** ≤ 48 B.

### `ObjectEntry` / `ObjectRow`

| Field | Kelley | Target |
|-------|--------|--------|
| `key` | IDX, ARENA | `key_off: StrOff` (L1) |
| `database_name` | IDX | `db_id: u16` (L2) |
| `script_id`, `checksum` | IDX, SOA | keep |
| `kind` | TAG | `ObjectRow.kind_code` |

**Steps:** L1–L3. **Target:** ~46–50 B/row.

### `Script`

| Field | Kelley | Target |
|-------|--------|--------|
| many `SharedStr` | DER, ARENA | `ScriptRow` 12–16 B (S1–S4) |

### `Workspace`

| Component | Kelley | Target |
|-----------|--------|--------|
| hot shell | SOA, COLD | `object_entries`, `object_keys`, `object_rows` inline (**W4–W5**) |
| cold slab | COLD, SLAB | `Box<WorkspaceCold>`: scripts, maps, arena, caches (**W4**) |
| caches | ARENA, IDX | `Vec<StrOff>` / `HashMap<u32, Vec<StrOff>>` (W1–W2) |
| maps | SPARSE | `transitions_by_row`, `parent_by_row` (W3) |

## Phase order (strict)

```text
0 → P1..P6 → A1..A3 + L1..L3 → S1..S4 → W1..W6 → C1 (optional)
```

## Verification commands

```bash
make rust-bench-footprint
cargo test -p migrator-core --test rust_footprint_baseline footprint_baseline_match -q
make rust-bench-footprint-alloc
make rust-bench-footprint-alloc ARGS=scan
make rust-bench-footprint-alloc ARGS=transitions
make go-rust-e2e
```

## Anti-drift (PR checklist)

- [ ] Step IDs listed in PR description
- [ ] Status table updated in this file
- [ ] dhat loop 0 B/iter (skip-heavy) or explained
- [ ] No new hot-path fat `PlannedObject` storage
- [ ] CASE-* cited from policy doc when layout changes

## References

- [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md)
- [`perf-footprint-audit.md`](perf-footprint-audit.md)
- [Andrew Kelley — Practical Data Oriented Design](https://www.youtube.com/watch?v=IroPQ150F6c)
