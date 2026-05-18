# Benchmark delta: audit baseline vs post-change (2026-05-18)

## TL;DR — where improvements show up (and where they do not)

If you only look at **`ns/op` on `ExecuteTxBatch`**, you will see **`benchstat` report `~` (noise)** — that is expected at `n=5` and does **not** mean the change did nothing.

| Benchmark | `ns/op` (benchstat) | `B/op` | `allocs/op` |
| --- | --- | --- | --- |
| `CollectStatements_500Transactional` | **−10.76%** (`p=0.008`) | **−50.05%** (`p=0.008`) | **−3.91%** (`p=0.008`) |
| `ExecuteTxBatch_SuccessPath_100Statements` | **~** (not significant) | **−11.99%** (`p=0.008`) | **−6.31%** (`p=0.008`) |
| `ExecuteTxBatch_FailurePath_100Statements` | **~** (not significant) | **−6.10%** (`p=0.008`) | **−3.08%** (`p=0.008`) |

**Plain reading:** memory per iteration (**`B/op`**) and heap allocation count (**`allocs/op`**) dropped on the apply micro-benchmarks; **wall time per op** for the transaction-batch benches did not prove a win in this capture.

**Why a full `rmig` run may feel unchanged:** real runs spend most time in **SQL Server**, **network**, and **reading files** — not in building Go maps for path lookup or in the apply batch string builder.

## What this is

A **`benchstat`** comparison between:

1. **Baseline** — excerpt from `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt` (same harness: `darwin/arm64`, `cpu: Apple M2`, `-count=5`, `-benchtime=150ms`, same `-bench` regex for `internal/bus`, `internal/db`, `internal/fs`, `internal/apply`).
2. **Post** — fresh `go test` on the current tree with the **identical** flags and package list, written to `bench-audit-post.txt`.

## Why it exists

Answers “**what changed in numbers vs the saved audit run?**” without relying on chat. This is **not** a strict git parent/child comparison (see constraints below).

## How to reproduce

```bash
DOC=docs/perf/runs/2026-05-18-baseline-vs-post
sed -n '4,92p' docs/perf/runs/2026-05-18-audit-bench/bench-output.txt > "$DOC/bench-audit-baseline.txt"

go test ./internal/bus ./internal/db ./internal/fs ./internal/apply -run '^$' \
  -bench '^BenchmarkBusPublish_|BenchmarkScopeKey_|BenchmarkBuildDualINQuery_|BenchmarkInspectorInspect_|BenchmarkScannerPreloadGitInfo_|BenchmarkCollectStatements_|BenchmarkExecuteTxBatch_' \
  -benchmem -count=5 -benchtime=150ms 2>&1 | tee "$DOC/bench-audit-post.txt"

"$(go env GOPATH)/bin/benchstat" "$DOC/bench-audit-baseline.txt" "$DOC/bench-audit-post.txt" | tee "$DOC/benchstat-audit-full.txt"
```

Apply-only slice + `benchstat`:

```bash
DOC=docs/perf/runs/2026-05-18-baseline-vs-post
awk '/^pkg: reporting-db-migrations\/internal\/apply$/,/^PASS$/' "$DOC/bench-audit-baseline.txt" | sed '/^PASS$/d' > "$DOC/bench-apply-baseline.txt"
awk '/^pkg: reporting-db-migrations\/internal\/apply$/,/^PASS$/' "$DOC/bench-audit-post.txt" | sed '/^PASS$/d' > "$DOC/bench-apply-post.txt"
"$(go env GOPATH)/bin/benchstat" "$DOC/bench-apply-baseline.txt" "$DOC/bench-apply-post.txt" | tee "$DOC/benchstat-apply-only.txt"
```

**Install `benchstat`:** `go install golang.org/x/perf/cmd/benchstat@latest`

## Measured deltas (same machine class, `n=5`)

Summarized from `benchstat-apply-only.txt` in this directory:

| Benchmark | ns/op vs baseline | B/op vs baseline | allocs/op vs baseline |
| --- | --- | --- | --- |
| `BenchmarkCollectStatements_500Transactional` | **−10.76%** (`p=0.008`) | **−50.05%** (`p=0.008`) | **−3.91%** (`p=0.008`) |
| `BenchmarkExecuteTxBatch_SuccessPath_100Statements` | ~ (noise) | **−11.99%** (`p=0.008`) | **−6.31%** (`p=0.008`) |
| `BenchmarkExecuteTxBatch_FailurePath_100Statements` | ~ (noise) | **−6.10%** (`p=0.008`) | **−3.08%** (`p=0.008`) |

`benchstat` prints `± ∞` here because `n=5` is below the threshold for full confidence intervals at 0.95; treat wall-time `~` rows as **not proven faster/slower** despite point movement.

**New benchmark (no baseline row):** `BenchmarkLayoutRebuildPathIndexes_500Objects` — see `bench-layout-rebuild-post.txt` in this directory (`~11.3 µs/op`, `~27.4 KiB/op`, `5 allocs/op` on the capture machine).

## Constraints

- Baseline is a **checked-in historical stdout**, not necessarily the immediate git parent of the post-change commit.
- Re-running on a **different CPU or OS** changes absolute numbers; re-capture both legs on the same machine before claiming a regression win.
- `benchstat` applies **paired comparison** only when benchmark names and units line up across the two files.

## Files

| File | Meaning |
| --- | --- |
| `bench-audit-baseline.txt` | Baseline excerpt from the 2026-05-18 audit bench run. |
| `bench-audit-post.txt` | Post-change `go test` output (full four packages). |
| `bench-apply-baseline.txt` / `bench-apply-post.txt` | `internal/apply` slices for focused `benchstat`. |
| `benchstat-audit-full.txt` | `benchstat` across all benchmarks present in both files. |
| `benchstat-apply-only.txt` | `benchstat` for `internal/apply` only. |
| `bench-layout-rebuild-post.txt` | Post-only numbers for `BenchmarkLayoutRebuildPathIndexes_500Objects`. |

## References

- `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt` (source of baseline excerpt)
- `docs/internal-performance-audit.md`
- `docs/perf/runs/2026-05-18-apply-fs-profile/README.md` (`pprof` evidence)
