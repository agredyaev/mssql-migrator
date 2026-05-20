MERGE azdo_deploy_meta.catalog_meta AS t
USING (SELECT 1 AS id) AS s ON t.id = s.id
WHEN MATCHED THEN
    UPDATE SET layout_digest = @p1, object_count = @p2, captured_at = SYSUTCDATETIME()
WHEN NOT MATCHED THEN
    INSERT (id, layout_digest, object_count, captured_at)
    VALUES (1, @p1, @p2, SYSUTCDATETIME());
