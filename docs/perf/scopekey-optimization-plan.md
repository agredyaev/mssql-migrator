# Technical Document: `scopeKey` optimization plan

Lifecycle: `Current`.

## Purpose

This document is the **single place** that explains what `scopeKey` does for performance, what has already been changed, what still dominates allocations (if anything), and **what to try next** (in order), with exact paths and how to verify changes. It exists so reviewers do not have to reconstruct intent from scattered benchmarks or chat.

## Scope

- Implementation: `internal/db/inspector_impl.go` (`scopeKey`, `scopePart`, `scopeKeySHA256Hex`)
- Consumers: `internal/db/inspector_impl.go` (`Inspect` cache keyed by SHA-256 hex of canonical `scopeKey`, or `""` for empty layout)
- Benchmarks: `internal/db/inspector_bench_test.go` (`BenchmarkScopeKey_2000Parts`, `BenchmarkScopeKeyPhase3SlotKey_2000Parts`)
- Commands:

```bash
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKey_2000Parts$' -benchmem -count=5
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKeyPhase3SlotKey_2000Parts$' -benchmem -count=5
```

Out of scope: changing MSSQL wire protocol, inspector query SQL, or cache correctness without tests.

## System Context

`scopeKey` builds a **deterministic canonical string** from every schema, object, transition, and check path in an `fs.Layout`. Golden tests assert that canonical string **byte-for-byte**.

**Phase 3 (`Inspect`):** the per-inspector `cache` map and the shared cache key suffix use **`scopeKeySHA256Hex(canonical)`** — `hex(SHA256([]byte(canonical)))` for non-empty canonical, and **`""`** when the canonical string is empty (empty layout). This shortens long-lived map keys while preserving collision resistance. Canonical `scopeKey` text is still computed on every `Inspect` (Variant 2); Variant 3 (stream hash without full string) remains a future option.

The canonical key **must** be stable for the same layout and **must** differ when any tracked path changes. That is enforced by collecting labeled fragments (`s:…`, `o:…`, `t:…`, `c:…`), **sorting** them in the same **lexicographic order** as the historical `"kind:"+payload` strings, and joining with `|`.

## Interfaces And Boundaries

- Inputs: `fs.Layout` (schemas, object pointers, transition pointers, check pointers).
- Outputs: (1) canonical `scopeKey` string (goldens, digest input); (2) **64-char hex** digest (or `""`) for `(*inspector).cache` map keys and the third segment of `sharedScopeCacheKey`.
- Boundaries: callers treat the digest as opaque; only equality matters. Changing digest input is a **breaking internal contract** for in-memory caches (same as changing canonical `scopeKey` bytes).

## Assumptions And Constraints

- Assumptions: `NormalizedName`, `NormalizedKey`, and `Path` fields already carry the canonical strings used elsewhere in the engine.
- Constraints: canonical `scopeKey` strings **must** stay **byte-for-byte** identical to the legacy `[]string` + `sort.Strings` + `strings.Join` contract for every layout (golden tests). The inspector **cache slot key** is allowed to change format as long as it remains **injective** with respect to canonical strings (SHA-256 hex satisfies this in practice).

## Nominal Flow (current code)

1. Let `n = len(Schemas)+len(Objects)+len(Transitions)+len(Checks)`.
2. If `n == 0`, return `""`.
3. Build `parts := make([]scopePart, 0, n)` where each `scopePart` is `{ kind: 's'|'o'|'t'|'c', s: payload }` referencing existing `string` data (no `"x:"+s"` allocation per row).
4. `sort.Slice(parts, …)` with **tuple compare**: `kind` byte first, then `s` — matches lexicographic order of `"kind:"+s"` because each `kind` is a single ASCII letter and the second byte of the legacy token is always `':'`.
5. One `strings.Builder` pass: `Grow` estimate from lengths, then for each part in order write `kind`, `':'`, `payload`, and `|` between entries.
6. **`Inspect`:** `canonical := scopeKey(scope)`, `slotKey := scopeKeySHA256Hex(canonical)`, `sharedScopeCacheKey(conn, slotKey)`; `d.cache[slotKey]` holds `*cachedScope`.

## Measured baseline (what the benchmark shows)

Run:

```bash
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKey_2000Parts$' -benchmem -count=5
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKeyPhase3SlotKey_2000Parts$' -benchmem -count=5
```

On a recent `darwin/arm64` (`Apple M2`) capture with **`-benchtime=200ms`**, expect **roughly**:

- **~127–158 µs/op** (wall time is noisy),
- **~107 KiB/op**,
- **~5 allocs/op** for **`largeBenchLayout(1, 2000, 0, 0)`** — one schema row plus 2000 object rows (see `internal/db/inspector_bench_test.go`).

A **`count=5`** capture on the same class of machine (2026-05-18 gate) reported **~93–95 µs/op** with the same **~106 KiB/op** and **5 allocs/op** — treat `ns/op` as environment-sensitive; `B/op` and `allocs/op` are the stable signals here.

**Phase 3 combined path** (`scopeKey` + `scopeKeySHA256Hex`, `BenchmarkScopeKeyPhase3SlotKey_2000Parts`, `count=3` on `Apple M2`): expect **roughly** **~8 allocs/op** and **~164 KiB/op** in addition to higher `ns/op` versus canonical-only — digest adds hashing and a 64-byte string allocation; the win is **shorter map keys** for large scopes, not lower micro-bench alloc count on this harness.

Those numbers are **fixture-sized**, not a universal constant.

## Where the allocations come from (after Phase 2)

The benchmark’s **~5 allocs/op** (canonical-only `BenchmarkScopeKey_2000Parts`) are dominated by the **`[]scopePart` backing array**, **`sort.Slice`** auxiliary cost, and the **`strings.Builder`** / final **`string`** materialization — not by **O(n)** per-row `prefix+payload` strings (those are gone). The **Phase 3** digest adds a few more allocations on the hot path (see `BenchmarkScopeKeyPhase3SlotKey_2000Parts`).

## Phased plan (status)

### Phase 0 — Done

- Pre-sized slice / empty-layout fast path (historical; superseded by Phase 2 structure but constraints retained).
- Contract: `internal/db/inspector_test.go` (`TestScopeKeyEmptyLayout`).

### Phase 1 — Done

- `benchmem` before/after recorded in this document and `docs/internal-performance-audit.md` spot-check lines.

### Phase 2 — Done (`scopePart` + `sort.Slice` + `strings.Builder`)

- Implemented in `internal/db/inspector_impl.go`.
- Golden coverage: `internal/db/inspector_test.go` (`TestScopeKeyGoldenMixed`).
- **Exit met:** `allocs/op` dropped from **~2003** to **~5** on `BenchmarkScopeKey_2000Parts`; `go test ./...` passes.

### Gate A.0 — Phase 3 go / no-go (historical record)

Earlier cycle: **no-go** purely on micro-benchmark cost alone (see table in git history around 2026-05-18). The gate was **re-opened** when Phase 3 was implemented for **long-lived map key size** and shared cache key payload, accepting extra digest work per `Inspect`.

### Phase 3 — Done (Variant 2: SHA-256 hex of canonical UTF-8)

- **Code:** `scopeKeySHA256Hex` in `internal/db/inspector_impl.go`; `(*inspector).Inspect` keys `d.cache` by digest and passes digest into `sharedScopeCacheKey`. Empty canonical still uses `""` digest (empty layout unchanged).
- **Tests:** `TestScopeKeySHA256HexEmptyCanonical`, `TestScopeKeySHA256HexGoldenVectors`; fuzz asserts digest idempotency; goldens for canonical `scopeKey` unchanged.
- **Bench:** `BenchmarkScopeKeyPhase3SlotKey_2000Parts` measures canonical + digest.
- **Breaking internal contract:** any in-process `inspector` built before this change would use different `cache` keys; cross-version in one process is unsupported. `InvalidateInspectorCache` generation bump still applies per connection stable key.

#### Variant 3 (not started)

Stream-hash sorted parts without materializing the full canonical string — only if profiling shows canonical string allocation dominates on multi-megabyte scopes. Equivalence test: `streamDigest(layout)` vs `SHA256([]byte(scopeKey(layout)))`.

## Verification And Validation

- Always: `go test ./internal/db ./internal/diff ./internal/engine -count=1` (or full `go test ./...`).
- Perf: `go test ./internal/db -run '^$' -bench '^BenchmarkScopeKey_2000Parts$' -benchmem -count=5` and `BenchmarkScopeKeyPhase3SlotKey_2000Parts`.
- Regression: golden canonical strings in `internal/db/inspector_test.go`; digest vectors in `TestScopeKeySHA256HexGoldenVectors`.

## Operations And Recovery

- Routine: when touching `scopeKey`, update **this** document’s measured numbers in the same change if benchmarks move.
- Rollback: revert the PR; cache keys change if string format changes — treat any format change as a **breaking internal contract** unless versioned.

## Open Issues And Non-Goals

- Open issues: `BenchmarkScopeKey_2000Parts` uses `largeBenchLayout(1, 2000, 0, 0)` — **one schema + 2000 objects**, **zero** transitions and checks. The benchmark name is historical; measured `allocs/op` tracks that shape, not arbitrary mixed layouts.
- Non-goals: promising wall-time wins on `Inspect` end-to-end without DB I/O profiles; micro-optimizing `scopeKey` below the noise floor of `benchstat` without `n≥5`.

## References

- Code: `internal/db/inspector_impl.go` (`scopeKey`, `scopePart`, `scopeKeySHA256Hex`)
- Tests: `internal/db/inspector_test.go` (`TestScopeKeyEmptyLayout`, `TestScopeKeyGoldenMixed`, `TestScopeKeyGoldenWithAllPartKinds`, `TestScopeKeySHA256Hex*`); fuzz `internal/db/inspector_fuzz_test.go` (`FuzzScopeKey_stringStableUnderRepeat`, `FuzzScopeKey_digestMatchesSHA256`)
- Bench: `internal/db/inspector_bench_test.go` (`BenchmarkScopeKey_2000Parts`, `BenchmarkScopeKeyPhase3SlotKey_2000Parts`, `largeBenchLayout`)
- Index: `docs/perf/README.md`
- Broader audit: `docs/internal-performance-audit.md`
