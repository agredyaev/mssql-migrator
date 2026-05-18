# Technical Document: Module `internal/types`

Lifecycle: `Current`.

## Purpose

Describe **shared domain types**: configuration, migration plan structures, checksum and key helpers, JSON compatibility for plan artifacts, and event names.

## Scope

- `internal/types/config.go` — `Config`, duration fields, auth modes, transaction/update policy constants
- `internal/types/plan.go`, `internal/types/planned_json.go` — `MigrationPlan`, `PlannedObject`, JSON marshaling rules
- `internal/types/key.go` — `NormalizedKey` for map keys
- `internal/types/chunk.go` — SQL `IN` list chunking, `BuildDualINQuery`
- `internal/types/events.go` — bus event identifiers and payload structs
- `internal/types/report.go` — report-related structs if present
- Tests and fuzz targets under `internal/types/`

## System context

Every major package imports `types` for configuration and plan data. `Config` is built exclusively in `internal/app/config.go` today.

## Interfaces and boundaries

- Inputs: stringly environment and flag data (converted in `app`)
- Outputs: strongly typed values consumed across `engine`, `diff`, `apply`, `audit`, `report`
- JSON tags on plan types define on-disk plan format compatibility

## Assumptions and constraints

- Assumption: changing `PlannedObject` JSON fields is a **breaking** contract for external consumers; update docs and golden tests together.
- Constraint: exit codes live alongside config in `types` (see `Exit*` constants).

## Nominal flow

Types are passive data; no background goroutines.

## Off-nominal behavior and failure containment

- Validation errors for config happen in `app` before structs are used for I/O.

## Verification and validation

- `make check` (includes `internal/types` tests and fuzz build tags as applicable)

## Operations and recovery

- When adding `RM_*` variables: extend `internal/app/config.go` mapping, `validateConfig` when the variable is required, and **`docs/solution.md`** / **`docs/operational-contract.md`** if operators must set it.

## Open issues and non-goals

- Non-goals: `types` does not perform database or filesystem I/O.

## References

- `internal/app/config.go`
- `docs/specs/internals/module-app.md`
