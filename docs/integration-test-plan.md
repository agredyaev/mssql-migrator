# MSSQL Integration Test Plan

Run against a disposable SQL Server database.

1. Bootstrap `__migrator.schema_migrations`.
2. Apply `V001`.
3. Re-run and confirm `V001` is skipped.
4. Modify applied `V001` and confirm checksum mismatch.
5. Apply `R001`.
6. Modify `R001` and confirm repeatable rerun.
7. Create broken view and confirm validation fails.
8. Add failing check script and confirm validation fails.
9. Start two migrations and confirm app lock blocks the second.
10. Change `sql_dir_hash` after planning and confirm migration is blocked.
11. Baseline historical `V` scripts up to a supplied version and confirm history rows are written.
12. Repair a stored checksum for an already applied script and confirm the metadata row updates.
