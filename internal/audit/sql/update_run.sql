UPDATE __migrator.runs
SET finished_at = @p1, result = @p2, exit_code = @p3
WHERE run_id = @p4
