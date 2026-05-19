# Technical Document: CI checkout for git delta and scoped inspect

Lifecycle: `Current`.

## Purpose

Describe **what the CI pipeline must provide** so `rmig` can resolve SQL file deltas and run scoped catalog inspect **without** manual `RMIG_GATE_*` variables.

## Scope

- Git delta: [`internal/prodgate/changed_paths.go`](../internal/prodgate/changed_paths.go) (`ResolveChangedPaths`)
- Scoped inspect: [`internal/engine/inspect_scope.go`](../internal/engine/inspect_scope.go), [`docs/prod-gate.md`](prod-gate.md)
- Applies to: `rmig plan`, `rmig migrate`, `make test-prod-gate`

## System context

`rmig` discovers the repository root via `.git` walking upward from `RM_SQL_ROOT`. It then resolves changed paths from **standard CI environment variables** (pull request base SHA) or, locally, `git merge-base HEAD origin/main` (or `main` / `master`).

If git resolution fails, the tool falls back to **full catalog inspect** (safe, slower).

## Interfaces and boundaries

### Pipeline inputs (automatic)

| Provider | Required checkout / env | Delta source |
|----------|-------------------------|----------------|
| **GitHub Actions PR** | `fetch-depth: 0` (or fetch base ref); `GITHUB_EVENT_PATH` with `pull_request.base.sha` | `git diff base HEAD --name-only` |
| **GitLab MR** | `CI_MERGE_REQUEST_DIFF_BASE_SHA`, `CI_COMMIT_SHA` | `git diff $BASE $SHA --name-only` |
| **Azure DevOps PR** | `fetchDepth: 0` or fetch `origin/<target>`; `SYSTEM_PULLREQUEST_TARGETBRANCH` | `merge-base` + diff |
| **Local dev** | Full clone; feature branch vs `main` | `merge-base HEAD main` |

### Not required in production pipelines

| Variable | Role |
|----------|------|
| `RMIG_GATE_GIT_BASE` | Test / local override only |
| `RMIG_GATE_CHANGED_FILES` | Test / local override only |
| `RMIG_GATE_*` (except harness toggles below) | Not part of promotion contract |

### Harness-only (integration / perf)

| Variable | Purpose |
|----------|---------|
| `RMIG_GATE_SKIP_DB_RESET=1` | Warm prod gate (no DROP/CREATE) |
| `RMIG_GATE_REPORT` | Write gate JSON path |
| `RMIG_GATE_UPDATE_BASELINE=1` | Maintainer baseline refresh |
| `RMIG_GATE_MAX_PLAN_WALL_MS` | Optional plan wall SLO |

### Force full inspect (escape hatches)

| Variable | Effect |
|----------|--------|
| `RM_SKIP_GIT=1` | No git metadata in scan; full catalog inspect |
| `RMIG_INSPECT_FULL=1` | Full catalog inspect despite git |

## Nominal flow

```yaml
# Example: GitHub Actions (conceptual)
steps:
  - uses: actions/checkout@v4
    with:
      fetch-depth: 0
  - run: rmig --env .env plan
```

No `export RMIG_GATE_GIT_BASE=...` step.

## Off-nominal behavior

- **Shallow clone without base ref:** `merge-base` fails → full inspect; gate with empty delta requires exact baseline match.
- **Running on `main` after merge:** delta often empty → scoped inspect uses stable keys + history; strict gate compares full snapshot to baseline.
- **No `.git` in workspace:** full inspect (`source=no-git`).

## Verification

- Unit: `go test ./internal/prodgate/ -run TestResolveChangedPaths`
- Integration: `make test-prod-gate` on a PR branch or temp git fixture

## References

- [`docs/prod-gate.md`](prod-gate.md)
- [`docs/solution.md`](solution.md) — scoped inspect (phase 1)
- [`ops/perf/README.md`](../ops/perf/README.md)
