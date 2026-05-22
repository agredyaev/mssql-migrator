# Technical Document: Data-oriented layout invariants

Lifecycle: `Current`.

## Purpose

Record the **current** in-memory layout contract for the Rust plan pipeline (scan → diff → plan). Policy rules live in [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md); measurement in [`perf-footprint-audit.md`](perf-footprint-audit.md).

## Scope

- Hot path: [`crates/core/src/domain/`](../crates/core/src/domain/), [`crates/core/src/plan/`](../crates/core/src/plan/), [`crates/core/src/export/`](../crates/core/src/export/)
- Verification: [`ops/perf/`](../ops/perf/), [`crates/core/tests/testdata/perf/footprint_baseline.json`](../crates/core/tests/testdata/perf/footprint_baseline.json)

**Out of scope:** CI perf gates; SQL wall-time SLO.

## System context

Layout choices apply to code that runs once per catalog object on every plan/diff. The workspace uses dense row indices, a string arena, hot/cold split (`Workspace` / `WorkspaceCold`), and VIEW materialization at JSON export. See [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md) for CASE-* rules.

## Interfaces and boundaries

| Layer | Role |
|-------|------|
| `domain/` | Scan ingest, arena, object/script rows |
| `plan/` | Diff and scenario dispatch |
| `export/` | Wire `PlannedObject` / plan JSON (VIEW boundary) |
| `migrator-core-dev` | Benches and footprint baseline (not linked from `rmig` / `rmigd`) |

## Assumptions and constraints

- Diff iterates `for i in 0..object_count` by row index (**CASE-1**).
- No `SharedStr::new` in per-object diff loops after scan finalize.
- No full-scan `HashMap` iteration in diff (**DOD-X2**).
- Plan JSON wire shape unchanged unless a maintainer refreshes e2e baselines.

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

## Nominal flow

1. Scan finalize builds dense rows + arena strings.
2. Plan DB phase fills catalog/checksum side state.
3. Diff writes slim plan rows; export materializes wire objects for JSON.

## Off-nominal behavior

- Layout regression: warmed skip-heavy 5k dhat loop phase reports **> 0 B/iter** → treat as blocker until explained.
- Struct size drift vs committed baseline → `footprint_baseline_match` fails; refresh baseline only with intent (`make bench-footprint-update-baseline`).

## Verification

Committed struct baseline (darwin/arm64, [`footprint_baseline.json`](../crates/core/tests/testdata/perf/footprint_baseline.json)):

| Type | `size_of` (B) |
|------|--------------:|
| `Config` | 144 |
| `ConfigCold` | 232 |
| `Workspace` (hot) | 88 |
| `WorkspaceCold` | 928 |
| `ObjectEntry` | 48 |
| `PlannedObject` | 144 |
| `MigrationPlan` | 304 |

Skip-heavy diff @ 5k: dhat **loop 0 B/iter** after warmup (see [`perf-footprint-audit.md`](perf-footprint-audit.md)).

```bash
make bench-footprint
cargo test -p migrator-core-dev --test footprint_baseline footprint_baseline_match -q
make bench-footprint-alloc
make e2e
```

## Operations and recovery

- After layout PR under `domain/` or `plan/`: run `make bench-footprint` and attach `footprint_bench.txt` or dhat summary when sizes change.
- Recovery from stale baselines: `make bench-footprint-update-baseline`, commit JSON, re-run `make e2e`.

## Open issues and non-goals

- Non-goals: enforcing layout policy via compiler plugin; hard CI thresholds on criterion timings.

## References

- [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md)
- [`perf-footprint-audit.md`](perf-footprint-audit.md)
- [`docs/specs/rust/module-domain.md`](specs/rust/module-domain.md)
