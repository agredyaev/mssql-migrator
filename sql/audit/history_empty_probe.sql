SELECT CASE
    WHEN EXISTS (SELECT TOP (1) 1 FROM azdo_deploy_meta.history)
    THEN CAST(1 AS bit)
    ELSE CAST(0 AS bit)
END AS has_rows
