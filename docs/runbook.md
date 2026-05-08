# Technical Document: Runbook

Lifecycle: `Current`.

## Purpose

This document explains how to operate `rmig` after a failed migration, failed validation, or metadata repair.

## Scope

- Failure handling for `rmig migrate`
- Failure handling for `rmig validate`
- Operational use of `rmig baseline`
- Operational use of `rmig repair-checksum`
- Report files in `reports/migration-report.*` and `reports/validation-report.*`

## System Context

The runbook applies when `rmig` has already built a report or has partially changed SQL Server state.
It assumes the operator can read `./reports` and can run the CLI against the target database.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

 - Inputs: `reports/migration-report.json`, `reports/migration-report.txt`, `reports/validation-report.json`, `reports/validation-report.txt`, SQL Server error text, `--env`, `--confirm`
- Outputs: repaired metadata rows, rerun plan artifacts, rerun validation results
- Ownership boundaries: SQL fixes belong in Git; metadata repair belongs to the controlled CLI path

## Assumptions And Constraints

- Applied `V` scripts are not edited in place.
- `baseline` and `repair-checksum` require `--confirm`.
- `repair-checksum` is only for already applied scripts.
- `migrate` requires an approved plan file.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.

## Nominal Flow

1. Read the report file in `./reports`.
2. Identify the failing script, failed check, or metadata row.
3. Fix the SQL or metadata issue in the correct Git path.
4. Re-run the relevant command.

## Off-Nominal Behavior And Failure Containment

- Migration failure: stop, inspect the report, and fix forward with a new `V` script or corrected `R` script.
- Metadata failure after SQL success: stop deployment and inspect database state before retrying.
- Validation failure: fix the broken object or check script, then re-run `validate`.
- Plan drift after approval: regenerate `plan` before retrying `migrate`.

## Verification And Validation

- `rmig plan --env prod`
- `rmig migrate --env prod --plan-file reports/migration-plan.json`
- `rmig validate --env prod`
- `rmig baseline --env prod --up-to V010 --confirm`
- `rmig repair-checksum --env prod --script R002__views.sql --confirm`

## Operations And Recovery

- `baseline`: use once per existing database after the target version is confirmed.
- `repair-checksum`: use only when the stored checksum must match the applied script.
- After either metadata repair path, re-run `plan`.
- If SQL succeeded but metadata repair is unsafe or blocked, stop and escalate with the report files and database state snapshot.

## Open Issues And Non-Goals

- Open issues: this runbook does not define external release approvals or SQL Server provisioning.
- Non-goals: this document does not describe feature development or schema design.

## References

- `README.md`
- `docs/solution.md`
- `docs/operational-contract.md`
- `docs/integration-test-plan.md`
