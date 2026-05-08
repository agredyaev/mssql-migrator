# Runbook

## Failed migration

1. Read `reports/migration-report.json`.
2. Check failed script and SQL Server error.
3. Do not edit applied `V` scripts.
4. Fix forward with a new `V` script or corrected `R` script.
5. Re-run `plan`, then `migrate`.

## Critical metadata failure

If SQL succeeded but metadata write failed, stop deployment and inspect database state manually before retrying.

## Validation failure

1. Read `reports/validation-report.json`.
2. Fix broken view/procedure/function or check script.
3. Re-run `validate`.

## Baseline and checksum repair

1. Run `baseline` only once per existing database after confirming the target version.
2. Use `--confirm` for `baseline` and `repair-checksum`.
3. Use `repair-checksum` only when the stored checksum must be corrected for an already applied script.
4. Re-run `plan` after any metadata repair.
